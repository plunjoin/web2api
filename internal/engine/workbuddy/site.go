// site.go 描述 WorkBuddy 的两个发行版（国服 copilot.tencent.com /
// 国际服 www.workbuddy.ai）以及它们各自的请求签名。
//
// WorkBuddy 是腾讯 CodeBuddy 的桌面 Agent。两个 build 的 API、登录入口、
// 模型目录、甚至 User-Agent 里的 applicationName 都不同；上游会按
// User-Agent / X-IDE-Type 等头识别「是不是官方客户端」，不匹配直接拒绝。
// 这里集中定义，避免散落在各处的字面量。
package workbuddy

import "strings"

// Site 是 WorkBuddy 的一个发行版。
type Site struct {
	// ID 是引擎内部与凭据里的站点标识：workbuddy / workbuddy-ai。
	ID string
	// Name 是展示名。
	Name string
	// Region 是区域标签：cn / intl。
	Region string
	// Endpoint 是 API 根地址。
	Endpoint string
	// Platform 是启动登录流程时的 platform 参数。
	Platform string
	// AuthID 是桌面端本地登录文件的名字前缀。
	AuthID string
	// App 是桌面端版本（UA 里的 WorkBuddy/<app> 与 X-IDE-Version）。
	App string
	// CLI 是桌面端内置 CLI 的版本（UA 里的 CLI/<cli>）。
	CLI string
	// AppName 是 UA 里的 applicationName（两个 build 不同）。
	AppName string
	// KeysURL 是该 build 创建 API Key 的页面。
	KeysURL string
	// Messages 表示该 build 是否在 Anthropic Messages API（/v1/messages）
	// 上提供 Claude 模型。国际服没有这条路由。
	Messages bool
	// Models 是内置兜底模型目录（账号真实权益以 /v3/config 为准）。
	Models []string
}

// 两个 build 的当前桌面端版本，取自 magpie 的 SITES（WorkBuddy 5.9.3）。
const (
	siteApp    = "5.9.3"
	siteCLIVer = "2.165.0"
)

// SiteCN 是国服（copilot.tencent.com）。
var SiteCN = Site{
	ID:       "workbuddy",
	Name:     "WorkBuddy",
	Region:   "cn",
	Endpoint: "https://copilot.tencent.com",
	Platform: "workbuddy",
	AuthID:   "workbuddy-desktop",
	App:      siteApp,
	CLI:      siteCLIVer,
	AppName:  "WorkBuddy",
	KeysURL:  "https://www.workbuddy.cn/profile/keys",
	Messages: true,
	Models:   cnModels,
}

// SiteIntl 是国际服（www.workbuddy.ai）。
var SiteIntl = Site{
	ID:       "workbuddy-ai",
	Name:     "WorkBuddy AI",
	Region:   "intl",
	Endpoint: "https://www.workbuddy.ai",
	Platform: "workbuddy-ai",
	AuthID:   "workbuddy-desktop-ai",
	App:      siteApp,
	CLI:      siteCLIVer,
	AppName:  "workbuddy-ai",
	KeysURL:  "https://www.workbuddy.ai/profile/keys",
	Messages: false,
	Models:   intlModels,
}

// EngineFor reports which provider an account belongs to.
// International accounts are workbuddy-ai; CN and legacy CodeBuddy accounts are workbuddy.
func EngineFor(c Credentials) string {
	endpoint, site := resolveSite(c)
	if IsLegacyCodeBuddy(endpoint) {
		return SiteCN.ID
	}
	return site.ID
}

// resolveSite reads only the site declared by the account, ignoring engine config.
func resolveSite(c Credentials) (string, Site) {
	if b := strings.TrimSpace(c.BaseURL); b != "" {
		b = strings.TrimRight(b, "/")
		switch {
		case strings.Contains(strings.ToLower(b), "workbuddy.ai"):
			return b, SiteIntl
		case IsLegacyCodeBuddy(b):
			return b, SiteCN
		case strings.Contains(strings.ToLower(b), "copilot.tencent.com"):
			return b, SiteCN
		}
	}
	// 地址不是两个发行版之一（测试桩、或自建反代）时，以账号声明的站点为准。
	if s := strings.TrimSpace(c.Site); s != "" {
		return "", SiteFor(s)
	}
	if b := strings.TrimSpace(c.BaseURL); b != "" {
		return strings.TrimRight(b, "/"), SiteForBaseURL(b)
	}
	return "", SiteCN
}

// knownEditionURL 报告地址是否明确指向两个发行版或旧 CodeBuddy 之一。
func knownEditionURL(base string) bool {
	b := strings.ToLower(strings.TrimSpace(base))
	return strings.Contains(b, "workbuddy.ai") || strings.Contains(b, "copilot.tencent.com") || IsLegacyCodeBuddy(b)
}

// SiteFor 按站点标识/别名取 Site。空值与未知值都落到国服。
func SiteFor(name string) Site {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "workbuddy-ai", "workbuddy_ai", "intl", "international", "ai", "www.workbuddy.ai", "global":
		return SiteIntl
	default:
		return SiteCN
	}
}

// SiteForBaseURL 按上游地址猜发行版。用于兼容旧配置/旧账号。
func SiteForBaseURL(base string) Site {
	b := strings.ToLower(strings.TrimSpace(base))
	switch {
	case strings.Contains(b, "workbuddy.ai"):
		return SiteIntl
	default:
		return SiteCN
	}
}

// IsLegacyCodeBuddy 判断地址是否还是旧的 CodeBuddy（www.codebuddy.ai）。
// 旧地址沿用 CLI 的登录方式与请求头，避免升级后把已有账号打死。
func IsLegacyCodeBuddy(base string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(base)), "codebuddy.ai")
}

// endpoint 返回该站点的 API 根（去掉末尾斜杠）。
func (s Site) endpoint() string { return strings.TrimRight(s.Endpoint, "/") }

// host 返回站点 host，用作 X-Domain 的默认值。
func (s Site) host() string {
	ep := s.endpoint()
	if i := strings.Index(ep, "://"); i >= 0 {
		ep = ep[i+3:]
	}
	if i := strings.IndexByte(ep, '/'); i >= 0 {
		ep = ep[:i]
	}
	return ep
}

// cliAgent 是 WorkBuddy 桌面 CLI 的 User-Agent。
func (s Site) cliAgent() string {
	return "WorkBuddy/" + s.App + " " + s.AppName + "/" + s.App + " CLI/" + s.CLI
}

// cnModels 是国服内置兜底目录（账号真实权益以 /v3/config 为准）。
var cnModels = []string{
	"auto",
	"hy4-preview-f", "hy3", "hy3-x",
	"deepseek-v4.1-flash", "deepseek-v4.1-flash-sg", "deepseek-v4-pro",
	"glm-5.3", "glm-5.3-flash", "glm-5.2", "glm-5.1", "glm-5v-turbo",
	"minimax-m3",
	"kimi-k3-1", "kimi-k2.7", "kimi-k2.6",
}

// intlModels 是国际服内置兜底目录。
var intlModels = []string{
	"default-model", "fast-model", "balanced-model", "primary-model", "deep-model",
	"gpt-5.5", "gpt-5.4", "gpt-5.3-codex",
	"gemini-3.1-pro", "gemini-3.5-flash",
	"glm-5.3", "glm-5.2",
	"hy3",
	"kimi-k3", "kimi-k2.6",
	"minimax-m3",
}

// legacyModels 是旧 CodeBuddy（www.codebuddy.ai）的内置目录。
// 只为还在用旧地址的账号保留，升级后不至于把它们的模型从目录里删掉。
var legacyModels = []string{
	"auto-chat", "claude-4.0", "claude-3.7", "gpt-5", "gpt-5-mini",
	"gemini-2.5-pro", "gemini-2.5-flash",
}

// LegacyModels 返回旧 CodeBuddy 目录（id → 展示名）。
func LegacyModels() []ModelEntry {
	out := make([]ModelEntry, 0, len(legacyModels))
	for _, id := range legacyModels {
		out = append(out, ModelEntry{ID: id, Name: id})
	}
	return out
}

// DefaultModels 保留旧导出名：默认取国服目录。
var DefaultModels = SiteCN.Models

// ModelEntry 是站点目录里的一项（id + 展示名）。展示名用于管理台。
type ModelEntry struct {
	ID   string
	Name string
	// Free 是内置目录兜底的免费标识（config 读不到时用）。真实免费标识以
	// /v3/config 的 credits / modelPromotions 为准。
	Free bool
}

// fallbackFree 是内置目录里已知不耗积分的模型（config 读不到时的兜底）。
// 只放明确免费、且不是「夜间限免」的模型；Hy4 preview 只在 23:00–8:00 免费，
// 不算免费（与 magpie 的 priceOf 一致）。
var fallbackFree = map[string]bool{
	"deepseek-v4.1-flash": true,
}

// SiteModels 返回站点目录（id → 展示名）。
func SiteModels(s Site) []ModelEntry {
	names := map[string]string{
		"auto": "Auto", "hy4-preview-f": "Hy4 preview", "hy3": "Hy3", "hy3-x": "Hy3-X",
		"deepseek-v4.1-flash": "Deepseek-V4.1-Flash", "deepseek-v4.1-flash-sg": "Deepseek-V4.1-Flash SG",
		"deepseek-v4-pro": "Deepseek-V4-Pro",
		"glm-5.3":         "GLM-5.3", "glm-5.3-flash": "GLM-5.3-Flash", "glm-5.2": "GLM-5.2",
		"glm-5.1": "GLM-5.1", "glm-5v-turbo": "GLM-5v-Turbo", "minimax-m3": "MiniMax-M3",
		"kimi-k3-1": "Kimi-K3", "kimi-k3": "Kimi-K3", "kimi-k2.7": "Kimi-K2.7-Code", "kimi-k2.6": "Kimi-K2.6",
		"default-model": "Default", "fast-model": "Fast", "balanced-model": "Balanced",
		"primary-model": "Primary", "deep-model": "Deep",
		"gpt-5.5": "GPT-5.5", "gpt-5.4": "GPT-5.4", "gpt-5.3-codex": "GPT-5.3-Codex",
		"gemini-3.1-pro": "Gemini-3.1-Pro", "gemini-3.5-flash": "Gemini-3.5-Flash",
	}
	out := make([]ModelEntry, 0, len(s.Models))
	for _, id := range s.Models {
		name := names[id]
		if name == "" {
			name = id
		}
		out = append(out, ModelEntry{ID: id, Name: name, Free: fallbackFree[id]})
	}
	return out
}
