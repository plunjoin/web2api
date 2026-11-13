// Package teleagent 实现 TeleAgent（中国电信天翼 AI「星辰超级智能体」/ TeleClaw）
// 账号池引擎。凭据是本机桌面端登录后的 JWT（无 refresh token），请求经两步
// HMAC-SHA256 签名发往上游 OpenAI 兼容聊天网关。
//
// 协议参考社区插件 opencode-teleagent-auth（github.com/anbulang/opencode-teleagent-auth）
// 与仓库内逆向文档 docs/guide/teleagent-auth.md。
package teleagent

import (
	"strings"

	"web2api/internal/model"
)

// 上游常量（与插件一致，见 docs/guide/teleagent-auth.md 与插件 lib/models.mjs）。
const (
	// Origin 是聊天网关与积分门户的根地址。签名请求只会发给它。
	Origin = "https://agent.teleai.com.cn"
	// APIPrefix 是聊天网关的路径前缀（签名用的 pathname 必须带它）。
	APIPrefix = "/superCowork/sapi"
	// ChatPath 是聊天网关相对 APIPrefix 的路径。
	ChatPath = "/api/v1/chat/completions"
	// ChatEndpoint 是聊天网关完整地址。
	ChatEndpoint = Origin + APIPrefix + ChatPath
	// RouteTarget 是 X-Route-Target 头的固定值。
	RouteTarget = "ops-gateway"
	// DefaultAppVersion 是读不到桌面端版本时的兜底值（本机实测 2.6.0 可用）。
	DefaultAppVersion = "2.6.0"
	// DefaultAPIKey 是桌面端内置的聊天网关常量（app.asar 里 `id:"new-api"` 锚点
	// 附近的 apiKey，对所有用户一致，等价于客户端的 client key）。浏览器登录导入
	// 没有 app.asar 可读时用它兜底；本地导入会从 asar 读到真实值覆盖。
	DefaultAPIKey = "sk-qvp4m6h9g20uM6PgYP7E"
	// LoginURL 是桌面端登录入口（浏览器登录导入用）。登录完成后会跳到
	// `Origin + APIPrefix + /cowork/auth?code=…&token=…`，token 在回调 URL 里。
	LoginURL = Origin + "/meter/teleagent-login/login"
)

// Credentials teleagent 引擎凭据（accounts.credentials JSON）。
// Token 是解密后的本机登录态字符串，形如 `<tenant>:<JWT>`；DeviceID / InstallID
// 来自 app-auth/device-meta.json，参与签名；APIKey 是桌面端内置网关常量。
type Credentials struct {
	Token      string `json:"token"`
	DeviceID   string `json:"device_id,omitempty"`
	InstallID  string `json:"install_id,omitempty"`
	APIKey     string `json:"api_key,omitempty"`
	AppVersion string `json:"app_version,omitempty"`
	Scope      string `json:"scope,omitempty"`  // 账号目录名 v1_public_<userId>
	Email      string `json:"email,omitempty"`  // 展示名（JWT accountName）
	Proxy      string `json:"proxy,omitempty"`  // 账号级代理，优先于引擎全局代理
	// AutoCheckIn 是否自动签到（false 关闭后台自动补签，但仍刷新积分余额）。
	// nil 表示默认开启（与桌面端每日签到一致）。
	AutoCheckIn *bool `json:"auto_checkin,omitempty"`
}

// AutoCheckInEnabled 报告该账号是否开启自动签到（缺省视为开启）。
func (c Credentials) AutoCheckInEnabled() bool {
	return c.AutoCheckIn == nil || *c.AutoCheckIn
}

// modelEntry 是内置模型目录的一项。
type modelEntry struct {
	id      string
	name    string
	context int
	output  int
	rate    string
	vision  bool
}

// DefaultModels 是 TeleAgent 内置的 5 个模型（顺序即选择器顺序）。
// 值与插件 lib/models.mjs 一致；图片能力为实测结论，非上游目录声明。
var DefaultModels = []modelEntry{
	{id: "chat-pro", name: "智能", context: 400_000, output: 65_536, rate: "x0.6", vision: true},
	{id: "chat-flagship", name: "旗舰", context: 400_000, output: 65_536, rate: "x1.0", vision: false},
	{id: "chat-flash", name: "极速", context: 400_000, output: 65_536, rate: "x0.2", vision: true},
	{id: "chat-lite", name: "轻量", context: 100_000, output: 16_384, rate: "x0.5", vision: false},
	{id: "chat-nano", name: "轻速", context: 100_000, output: 16_384, rate: "x0.1", vision: false},
}

// visionModels 支持图片输入的模型 id 集合。
var visionModels = func() map[string]bool {
	out := map[string]bool{}
	for _, m := range DefaultModels {
		if m.vision {
			out[m.id] = true
		}
	}
	return out
}()

// isKnownModel 判断模型 id 是否在目录内。上游对未知 id 会静默回落到别的模型，
// 所以必须在本地拦下，不把这种请求放到上游。
func isKnownModel(id string) bool {
	for _, m := range DefaultModels {
		if m.id == id {
			return true
		}
	}
	return false
}

// ListModels 返回模型目录（available 取决于是否有可用账号）。
func (e *Engine) ListModels() []model.ModelInfo {
	enabled := e.usableCount()
	out := make([]model.ModelInfo, 0, len(DefaultModels))
	for _, m := range DefaultModels {
		out = append(out, model.ModelInfo{
			ID:          m.id,
			Object:      "model",
			DisplayName: m.name + " · " + m.rate,
			Available:   enabled > 0,
			Engine:      e.name,
			Kind:        "chat",
			Rate:        0, // 倍率是积分倍率，非折扣；保持 0 避免被当作免费
		})
	}
	return out
}

// ---- 思考档位适配（与插件 lib/effort.mjs 一致） ----
//
// 上游只认 low / high / max 三档，且不接受「关闭思考」的写法。宿主可能按
// 自己的通用理解写 reasoning_effort（如 medium），这里在发请求前削成合法档位。

var acceptedEfforts = map[string]bool{"low": true, "high": true, "max": true}

var offEfforts = map[string]bool{
	"off": true, "none": true, "disabled": true, "false": true, "no": true, "0": true,
}

// normalizeEffort 把任意档位取值映射成上游认的档位。返回空串表示应删掉字段。
func normalizeEffort(level string) string {
	value := strings.ToLower(strings.TrimSpace(level))
	if value == "" || offEfforts[value] {
		return ""
	}
	if acceptedEfforts[value] {
		return value
	}
	switch value {
	case "minimal":
		return "low"
	case "medium", "xhigh":
		return "high"
	}
	return "high"
}
