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
	cookies := CookieHeader(account.cookies, "flow.google.com")
	boot, err := account.client.Bootstrap(ctx, account.projectID, cookies)
	if err != nil {
		var authErr *AuthenticationError
		if errors.As(err, &authErr) {
			e.markCooldown(account, authErr.Error(), 5*time.Minute)
		}
		e.setErr(err)
		return Bootstrap{}, "", err
	}
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
	if e.st != nil {
		if id, err := parseAccountID(account.id); err == nil {
			if stored, err := e.st.GetAccount(id); err == nil {
				var creds Credentials
				if json.Unmarshal([]byte(stored.Credentials), &creds) == nil {
					creds.ProjectID = projectID
					_ = e.st.UpdateAccountCredentials(id, engine.MarshalCompact(creds))
				}
			}
		}
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
