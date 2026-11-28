package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"web2api/internal/engine"
	"web2api/internal/model"
)

// pickAccount 为一次生成任务挑一个可用账号。
func (e *Engine) pickAccount(modelID string, kind engine.PolicyKind) (*flowAccount, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	ordered := append([]*flowAccount(nil), e.accounts...)
	e.order.Sort(ordered, func(account *flowAccount) string { return account.id })
	for _, account := range ordered {
		account.mu.Lock()
		usable := account.usableLocked()
		account.mu.Unlock()
		if usable && e.policyEnabledLocked(account, kind) {
			return account, nil
		}
	}
	return nil, engine.UnavailableError(e)
}

// withSession 初始化账号的 RPC 会话（引导、项目、浏览器指纹）。
//
// 每次调用都重新 bootstrap：Flow 的 f.sid / XSRF 与浏览器打码会话绑定，
// 复用旧引导信息容易被风控拒掉。
func (e *Engine) withSession(ctx context.Context, account *flowAccount, action string) (Bootstrap, string, error) {
	account.mu.Lock()
	before := append([]Cookie(nil), account.cookies...)
	storedCookies := account.creds.Cookies
	projectID := account.projectID
	account.mu.Unlock()
	boot, err := account.client.Bootstrap(ctx, projectID, before)
	if err != nil {
		var authErr *AuthenticationError
		if errors.As(err, &authErr) {
			e.markCooldown(account, authErr.Error(), 5*time.Minute)
		}
		e.setErr(err)
		return Bootstrap{}, "", err
	}
	if err := e.persistRefreshedCookies(account, before, storedCookies, boot); err != nil {
		e.markCooldown(account, err.Error(), 5*time.Minute)
		e.setErr(err)
		return Bootstrap{}, "", err
	}
	cookies := CookieHeader(boot.Cookies, "flow.google.com")
	if boot.ProjectID != "" {
		e.setAccountProjectID(account, boot.ProjectID)
	}
	if err := e.applyCaptcha(ctx, account, action); err != nil {
		e.setErr(err)
		return Bootstrap{}, "", err
	}
	if account.projectID == "" {
		projectID, err := e.ensureProject(ctx, account, boot, cookies)
		if err != nil {
			e.setErr(err)
			return Bootstrap{}, "", err
		}
		boot.ProjectID = projectID
	}
	return boot, cookies, nil
}

// persistRefreshedCookies 在项目页确认仍是登录态后，把轮换过的 Cookie 写回。
//
// 只续活着的会话：跳去登录页、Cookie 不完整、页面没有邮箱，都不会进到这里。
// 页面邮箱已被别的 Flow 账号占用时也不写，避免两个账号抢同一份登录态。
// 库里的 cookies 已经不是这次请求带出去的那份时，条件更新会落空，不覆盖并发的手动同步。
func (e *Engine) persistRefreshedCookies(account *flowAccount, before []Cookie, storedCookies string, boot Bootstrap) error {
	if err := (Credentials{}).validateCookies(boot.Cookies); err != nil {
		return err
	}
	email := strings.TrimSpace(boot.Email)
	if email == "" {
		return &AuthenticationError{Detail: "项目页没有登录邮箱"}
	}
	if owner := e.emailOwner(email, account.id); owner != "" {
		return &AuthenticationError{Detail: "页面邮箱已被账号 " + owner + " 占用"}
	}
	encoded, err := json.Marshal(boot.Cookies)
	if err != nil {
		return err
	}
	next := string(encoded)
	// 无变化时不写库。比较用解析后的集合，不用库里的原文：
	// 粘贴进来的 Cookie 头是字符串，续期后存的是 JSON 数组。
	unchanged := cookiesEqual(before, boot.Cookies) && strings.EqualFold(strings.TrimSpace(accountEmail(account)), email)
	if unchanged {
		return nil
	}
	account.mu.Lock()
	stillOurs := cookiesEqual(account.cookies, before) && account.creds.Cookies == storedCookies
	if !stillOurs {
		account.mu.Unlock()
		return nil
	}
	previousEmail := account.creds.Email
	account.cookies = append([]Cookie(nil), boot.Cookies...)
	account.creds.Cookies = next
	account.creds.Email = email
	snapshot := account.creds
	account.mu.Unlock()
	if e.st == nil {
		return nil
	}
	id, err := parseAccountID(account.id)
	if err != nil {
		revertCookies(account, before, storedCookies, previousEmail, boot.Cookies, next)
		return nil
	}
	// expected 必须是 credentials.cookies 的库存原文。重新 json.Marshal
	// 出来的数组对不上粘贴的 Cookie 头，条件更新会落空并把内存回滚。
	wrote, err := e.st.UpdateAccountCredentialsIfCookiesMatch(id, storedCookies, engine.MarshalCompact(snapshot))
	if err != nil || !wrote {
		revertCookies(account, before, storedCookies, previousEmail, boot.Cookies, next)
	}
	return err
}

// revertCookies 在库写失败或落空时把内存退回这次请求带出去的登录态。
// 期间若已被手动同步换掉，则不动。
func revertCookies(account *flowAccount, before []Cookie, storedCookies, previousEmail string, applied []Cookie, next string) {
	account.mu.Lock()
	defer account.mu.Unlock()
	if cookiesEqual(account.cookies, applied) && account.creds.Cookies == next {
		account.cookies = append([]Cookie(nil), before...)
		account.creds.Cookies = storedCookies
		account.creds.Email = previousEmail
	}
}

// emailOwner 返回已占用该邮箱的其他 Flow 账号 ID。没有则返回空。
func (e *Engine) emailOwner(email, exceptID string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return ""
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, other := range e.accounts {
		if other.id == exceptID {
			continue
		}
		other.mu.Lock()
		owned := strings.EqualFold(strings.TrimSpace(other.creds.Email), email)
		other.mu.Unlock()
		if owned {
			return other.id
		}
	}
	return ""
}

// accountEmail 读取账号记录里的邮箱。
func accountEmail(account *flowAccount) string {
	account.mu.Lock()
	defer account.mu.Unlock()
	return account.creds.Email
}

// cookiesEqual 比较两份 Cookie 集合是否逐条相同。
func cookiesEqual(left, right []Cookie) bool {
	a, errA := json.Marshal(left)
	b, errB := json.Marshal(right)
	return errA == nil && errB == nil && string(a) == string(b)
}

// applyCaptcha 取一个与该账号同浏览器的 reCAPTCHA token，并让 RPC 的 UA
// 与浏览器指纹保持一致。
func (e *Engine) applyCaptcha(ctx context.Context, account *flowAccount, action string) error {
	result, err := e.captcha.Token(ctx, account.projectID, action, account.cookies, e.proxyForAccount(account))
	if err != nil {
		return fmt.Errorf("获取 Flow reCAPTCHA token 失败: %w", err)
	}
	account.client.SetUserAgent(result.UserAgent)
	account.client.SetAcceptLanguage(result.AcceptLanguage)
	account.mu.Lock()
	account.recaptcha = result.Token
	account.mu.Unlock()
	return nil
}

// proxyForAccount 返回账号/引擎生效的代理。
func (e *Engine) proxyForAccount(account *flowAccount) string {
	return account.proxy(e.proxy)
}

// setAccountProjectID 把 bootstrap 发现的项目 ID 写回账号与凭据。
func parseAccountID(id string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(id), 10, 64)
}

func (e *Engine) setAccountProjectID(account *flowAccount, projectID string) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" || projectID == account.projectID {
		return
	}
	account.mu.Lock()
	account.projectID = projectID
	account.creds.ProjectID = projectID
	account.mu.Unlock()
	// 只补 project_id。整份凭据写回会把刚续上的 Cookie 盖回读到的旧值，
	// 也会把账号状态打回 initializing。
	if e.st == nil {
		return
	}
	if id, err := parseAccountID(account.id); err == nil {
		_ = e.st.SetAccountProjectID(id, projectID)
	}
}

// ensureProject 在账号还没有项目时新建一个 Flow 项目。
func (e *Engine) ensureProject(ctx context.Context, account *flowAccount, boot Bootstrap, cookies string) (string, error) {
	payload, err := account.client.Call(ctx, boot, rpcCreateProject, createProjectArgument("web2api"), cookies, "/projects")
	if err != nil {
		return "", err
	}
	projectID := FindUUID(payload)
	if projectID == "" {
		return "", errors.New("Flow 建项目响应里没有项目 ID")
	}
	e.setAccountProjectID(account, projectID)
	return projectID, nil
}

// chatPrompt 取最后一条用户消息作为生成提示词。
func chatPrompt(req model.ChatRequest) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		message := req.Messages[i]
		if message.Role != "user" {
			continue
		}
		text := ""
		switch content := message.Content.(type) {
		case string:
			text = strings.TrimSpace(content)
		case []model.ContentPart:
			for _, part := range content {
				if strings.EqualFold(part.Type, "text") && strings.TrimSpace(part.Text) != "" {
					text = strings.TrimSpace(part.Text)
					break
				}
			}
		}
		if text != "" {
			return text
		}
		if parts, ok := message.Content.([]model.ContentPart); ok {
			for _, part := range parts {
				if strings.EqualFold(part.Type, "text") && strings.TrimSpace(part.Text) != "" {
					return strings.TrimSpace(part.Text)
				}
			}
		}
	}
	return ""
}
