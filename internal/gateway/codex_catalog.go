package gateway

import (
	"net/http"
	"strings"
	"time"

	"web2api/internal/model"
)

// Codex 模型目录端点：GET /v1/codex/catalog
//
// 供 Codex 的 model_catalog_url（[model_providers.*] 表字段）拉取，
// 返回该 Key 当前可用的 chat 模型，格式与 cc-switch-model-catalog.json /
// ~/.codex/model_catalog.json 一致（snake_case）。号池加号、模型上下架或
// Key 白名单变化后，Codex 重新拉取即可看到最新列表，无需手改配置。
//
// 鉴权：Authorization: Bearer <sk-...> 或查询参数 ?key=<sk-...>。
// URL 内嵌 Key 是为了兼容不携带 Authorization 头的拉取方（CC Switch /
// Codex 对 model_catalog_url 的请求行为不一），Key 本就保存在同一份
// config.toml 里，不产生额外暴露。目录拉取不计 RPM、不记用量。
func (s *Server) handleCodexCatalog(w http.ResponseWriter, r *http.Request) {
	key := bearerKey(r)
	if key == "" {
		key = strings.TrimSpace(r.URL.Query().Get("key"))
	}
	var allowed func(string) bool
	if s.authDisabled() {
		allowed = func(string) bool { return true }
	} else {
		if key == "" {
			writeError(w, http.StatusUnauthorized, "Invalid API key", "authentication_error", nil)
			return
		}
		info, found, err := s.st.LookupKey(key)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "校验 Key 失败: "+err.Error(), "api_error", nil)
			return
		}
		if !found || !info.Enabled {
			writeError(w, http.StatusUnauthorized, "Invalid API key", "authentication_error", nil)
			return
		}
		if info.Expired(time.Now().Unix()) {
			writeKeyExpired(w, info.ExpiresAt)
			return
		}
		allowed = info.ModelAllowed
	}

	writeJSON(w, http.StatusOK, map[string]any{"models": s.codexCatalogModels(allowed)})
}

// codexCatalogModels 生成 Codex 目录条目。
//
// Google OpenAI 兼容端点（custom 引擎接 Gemini Key）的模型 ID 带 models/
// 前缀；Codex 用 slug 原样请求，网关路由两侧都会去前缀匹配，所以这里统一
// 去掉前缀再去重，否则 Gemini Key 的模型一个也进不了目录。
func (s *Server) codexCatalogModels(allowed func(string) bool) []map[string]any {
	out := []map[string]any{}
	seen := map[string]bool{}
	for _, m := range s.mgr.ListModels() {
		// 只导出对话模型（Codex 走 /v1/responses，生图/向量/音乐模型接不了）。
		if m.Kind != "chat" || !m.Available {
			continue
		}
		slug, ok := codexCatalogSlug(m.ID)
		if !ok || seen[strings.ToLower(slug)] {
			continue
		}
		if allowed != nil && !allowed(m.ID) && !allowed(slug) {
			continue
		}
		// 管理台显式下架的模型不进目录（与 keyguard 的口径一致：查询出错视为未管理）。
		if enabled, err := s.st.ModelEnabled(m.ID); err == nil && !enabled {
			continue
		}
		seen[strings.ToLower(slug)] = true
		m.ID = slug
		if m.DisplayName == "" || strings.HasPrefix(m.DisplayName, "models/") {
			m.DisplayName = slug
		}
		out = append(out, codexCatalogEntry(m))
	}
	return out
}

// codexCatalogEntry 把网关模型目录转换成 Codex 模型目录条目。
// 字段集合与 CC Switch 生成的 cc-switch-model-catalog.json（以及官方
// models_cache.json）保持同构：visibility 枚举只有 list/hide/none，
// support_verbosity 等字段为必填，缺失会导致 Codex 启动时解析失败。
func codexCatalogEntry(m model.ModelInfo) map[string]any {
	display := m.DisplayName
	if display == "" {
		display = m.ID
	}
	return map[string]any{
		"slug":                             m.ID,
		"display_name":                     display,
		"description":                      display + " · web2api",
		"context_window":                   1000000,
		"max_context_window":               1000000,
		"effective_context_window_percent": 95,
		"default_reasoning_level":          "medium",
		"default_reasoning_summary":        "none",
		"supported_reasoning_levels": []map[string]any{
			{"effort": "none", "description": "关闭额外推理"},
			{"effort": "low", "description": "较快的轻量推理"},
			{"effort": "medium", "description": "速度与推理深度均衡"},
			{"effort": "high", "description": "更深的推理"},
		},
		"shell_type":                     "unified_exec",
		"visibility":                     "list",
		"supported_in_api":               true,
		"priority":                       100,
		"input_modalities":               []string{"text", "image"},
		"base_instructions":              "You are Codex, a coding agent based on " + display + ". You and the user share the same workspace and collaborate to achieve the user's goals.",
		"supports_parallel_tool_calls":   true,
		"supports_reasoning_summaries":   true,
		"support_verbosity":              false,
		"supports_search_tool":           false,
		"supports_image_detail_original": false,
		"additional_speed_tiers":         []any{},
		"service_tiers":                  []any{},
		"experimental_supported_tools":   []any{},
		"truncation_policy":              map[string]any{"limit": 10000, "mode": "bytes"},
		"availability_nux":               nil,
		"upgrade":                        nil,
	}
}

// codexCatalogSlug 把网关模型 ID 转成 Codex 目录 slug：去掉 Google 的 models/
// 前缀；仍含 / 的（如 OpenRouter 的 vendor/model）Codex 选模型时无法区分，跳过。
func codexCatalogSlug(id string) (string, bool) {
	slug := strings.TrimPrefix(strings.TrimSpace(id), "models/")
	if slug == "" || strings.Contains(slug, "/") {
		return "", false
	}
	return slug, true
}
