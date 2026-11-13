// affinity.go 会话粘性：同一个会话尽量留在上次接单的账号上。
//
// 粘性与路由顺序是两个独立维度：顺序决定「正常情况下先走谁」，粘性只把
// 上次真正接单的账号提到该顺序的最前面。账号不可用或失败时照常换下一个，
// 成功后把映射刷新到实际接单的账号；这样可以保住上游 prompt cache，也让
// 多轮工具调用留在同一个账号的上下文里。
//
// 会话键优先取显式的 X-Session-Id / X-Conversation-Id 等请求头；没有时用
// 「system 消息 + 第一条 user 消息 + user 字段」的哈希近似，客户端每轮重发
// 完整历史时这个键保持稳定。
package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"web2api/internal/engine"
	"web2api/internal/model"
)

// 会话粘性模式。
const (
	AffinityOff     = "off"     // 关闭：每个请求都按路由顺序重新选
	AffinitySession = "session" // 会话内保持：上次接单的账号优先，失败仍可切换
)

const (
	affinityTTL        = 30 * time.Minute
	affinityMaxEntries = 20000
)

type affinityEntry struct {
	accountID string
	expires   time.Time
}

// affinityCache 保存会话键 → 账号 ID，模式与 TTL 一起受锁保护。
type affinityCache struct {
	mu      sync.Mutex
	mode    string
	entries map[string]affinityEntry
}

func (c *affinityCache) setMode(mode string) {
	c.mu.Lock()
	c.mode = mode
	if mode == AffinityOff {
		c.entries = nil
	}
	c.mu.Unlock()
}

func (c *affinityCache) currentMode() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mode == "" {
		return AffinitySession
	}
	return c.mode
}

func (c *affinityCache) lookup(key string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return ""
	}
	if time.Now().After(entry.expires) {
		delete(c.entries, key)
		return ""
	}
	return entry.accountID
}

func (c *affinityCache) store(key, accountID string) {
	if key == "" || accountID == "" {
		return
	}
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]affinityEntry)
	}
	if len(c.entries) >= affinityMaxEntries {
		for k, entry := range c.entries {
			if now.After(entry.expires) {
				delete(c.entries, k)
			}
		}
		if len(c.entries) >= affinityMaxEntries {
			// 仍然超限：放弃最早写入的一批（map 遍历顺序随机，够用即可）。
			for k := range c.entries {
				delete(c.entries, k)
				if len(c.entries) < affinityMaxEntries*3/4 {
					break
				}
			}
		}
	}
	c.entries[key] = affinityEntry{accountID: accountID, expires: now.Add(affinityTTL)}
}

// SetAffinityMode 保存会话粘性模式并清空旧映射。
func (m *Manager) SetAffinityMode(mode string) error {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case AffinityOff, AffinitySession:
	default:
		return fmt.Errorf("未知的会话粘性模式 %q（可选 off / session）", mode)
	}
	if m.st != nil {
		if err := m.st.SetRoutingAffinity(mode); err != nil {
			return err
		}
	}
	m.affinity.setMode(mode)
	return nil
}

// AffinityMode 当前会话粘性模式；未配置过时默认 session。
func (m *Manager) AffinityMode() string { return m.affinity.currentMode() }

// ReloadAffinity 从存储重新加载粘性模式（启动装配与保存后调用）。
func (m *Manager) ReloadAffinity() {
	if m.st == nil {
		m.affinity.setMode(AffinitySession)
		return
	}
	mode, err := m.st.RoutingAffinity()
	if err != nil {
		mode = AffinitySession
	}
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case AffinityOff:
		mode = AffinityOff
	default:
		mode = AffinitySession
	}
	m.affinity.setMode(mode)
}

// affinityContext 为本次请求准备账号跟踪与首选账号，返回更新后的 ctx、
// 会话键和首选账号 ID（没有映射时为空）。
func (m *Manager) affinityContext(ctx context.Context, req model.ChatRequest) (context.Context, string, string) {
	ctx = engine.WithAccountTracking(ctx)
	// 会话键不只用于粘性：引擎把它透传给上游作为会话 ID，从而复用
	// 上游的前缀缓存。所以即使关闭粘性也照常生成。
	key := conversationKey(ctx, req)
	if key != "" {
		ctx = engine.WithConversationID(ctx, key)
	}
	if m.affinity.currentMode() == AffinityOff || key == "" {
		return ctx, "", ""
	}
	accountID := m.affinity.lookup(key)
	if accountID != "" {
		ctx = engine.WithPreferredAccount(ctx, accountID)
	}
	return ctx, key, accountID
}

// noteAffinity 在请求成功后记录真正接单的账号；引擎没有上报时保留旧映射。
func (m *Manager) noteAffinity(key string, ctx context.Context) {
	if key == "" || m.affinity.currentMode() == AffinityOff {
		return
	}
	if served, ok := engine.ServedAccount(ctx); ok {
		m.affinity.store(key, served)
	}
}

// preferAffinityEngine 把粘性账号所属的供应商排到最前，保持其余顺序。
// 该供应商不在候选列表里（账号都不可用、或模型不属于它）时原样返回。
func (m *Manager) preferAffinityEngine(engines []engine.Engine, accountID string) []engine.Engine {
	if accountID == "" || len(engines) < 2 {
		return engines
	}
	name := m.engineForAccount(accountID)
	if name == "" {
		return engines
	}
	for _, eng := range engines {
		if eng.Name() == name {
			return moveEngineFirst(engines, eng)
		}
	}
	return engines
}

// sessionHint 由网关从请求头注入的显式会话标识。
type sessionHintKey struct{}

// WithSessionHint 绑定调用方提供的会话标识（X-Session-Id 等）。
func WithSessionHint(ctx context.Context, hint string) context.Context {
	hint = strings.TrimSpace(hint)
	if hint == "" {
		return ctx
	}
	return context.WithValue(ctx, sessionHintKey{}, hint)
}

func sessionHint(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	hint, _ := ctx.Value(sessionHintKey{}).(string)
	return strings.TrimSpace(hint)
}

// conversationKey 生成稳定的会话键。显式会话头优先；否则用 system 消息、
// 第一条 user 消息和 OpenAI 的 user 字段一起哈希，客户端每轮重发历史时
// 键保持不变。找不到 user 消息时返回空（不做粘性）。
func conversationKey(ctx context.Context, req model.ChatRequest) string {
	h := sha256.New()
	if hint := sessionHint(ctx); hint != "" {
		io.WriteString(h, "sid\x00")
		io.WriteString(h, hint)
	} else {
		io.WriteString(h, "conv\x00")
		io.WriteString(h, strings.TrimSpace(req.User))
		io.WriteString(h, "\x00")
		foundUser := false
		for _, msg := range req.Messages {
			role := strings.ToLower(strings.TrimSpace(msg.Role))
			switch {
			case role == "system" || role == "developer":
				writeKeyPart(h, role, msg.Content)
			case role == "user":
				writeKeyPart(h, role, msg.Content)
				foundUser = true
			}
			if foundUser {
				break
			}
		}
		if !foundUser {
			return ""
		}
	}
	return hex.EncodeToString(h.Sum(nil)[:16])
}

func writeKeyPart(h io.Writer, role string, content any) {
	io.WriteString(h, role)
	io.WriteString(h, "\x00")
	if s, ok := content.(string); ok {
		io.WriteString(h, s)
	} else if raw, err := json.Marshal(content); err == nil {
		h.Write(raw)
	}
	io.WriteString(h, "\x00")
}
