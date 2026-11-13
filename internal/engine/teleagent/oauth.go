// oauth.go 浏览器登录导入：把桌面端登录回调 URL 里的 token 提出来，
// 并为没有 device-meta.json 的浏览器登录生成设备/安装指纹。
package teleagent

import (
	"net/url"
	"strings"
)

// ExtractToken 从登录回调 URL 或裸 token 中提取 token。
// 桌面端登录后浏览器会跳到 `…/superCowork/sapi/cowork/auth?code=…&token=…`，
// token 就在查询参数里；用户也可能直接粘贴裸 token。
func ExtractToken(raw string) string {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "token=") {
		return raw
	}
	if u, err := url.Parse(raw); err == nil {
		if tok := u.Query().Get("token"); tok != "" {
			return tok
		}
	}
	if idx := strings.Index(raw, "token="); idx >= 0 {
		tok := raw[idx+len("token="):]
		if amp := strings.IndexAny(tok, "&?#"); amp >= 0 {
			tok = tok[:amp]
		}
		return tok
	}
	return raw
}

// NewDeviceID 生成设备指纹（浏览器登录用）。
func NewDeviceID() string { return newDeviceID() }

// NewInstallID 生成安装指纹（浏览器登录用）。
func NewInstallID() string { return newInstallID() }

// LoginSession 浏览器登录授权会话（authorize → exchange）。
type LoginSession struct {
	SessionID string
	AuthURL   string
}

// NewLoginSession 生成浏览器登录会话。TeleAgent 登录无需 PKCE/state，
// 登录完成后 token 直接在回调 URL 里，exchange 阶段直接提取、无需换码。
func NewLoginSession() *LoginSession {
	return &LoginSession{SessionID: newInstallID(), AuthURL: LoginURL}
}
