// Package admin 提供号池管理 REST API（/admin/api/*），
// 供 Web 管理台与外部脚本调用：账号 CRUD、Key 分发、用量统计。
// 鉴权：Authorization: Bearer <JWT>，登录会话存储在 Redis。
package admin

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"web2api/internal/provider"
	"web2api/internal/store"
	"web2api/internal/upgrade"
	"web2api/internal/version"
)

// API 管理接口。
type API struct {
	mgr     *provider.Manager
	st      *store.Store
	logger  *log.Logger
	updater *upgrade.Manager
}

// New 创建管理 API。
func New(mgr *provider.Manager, st *store.Store, logger *log.Logger, updaters ...*upgrade.Manager) *API {
	if logger == nil {
		logger = log.Default()
	}
	updater := upgrade.Disabled()
	if len(updaters) > 0 && updaters[0] != nil {
		updater = updaters[0]
	}
	return &API{mgr: mgr, st: st, logger: logger, updater: updater}
}

// Mount 注册到 mux。
func (a *API) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /admin/api/setup", a.handleSetupStatus)
	mux.HandleFunc("POST /admin/api/setup", a.handleSetup)
	mux.HandleFunc("POST /admin/api/auth/login", a.handleLogin)
	mux.HandleFunc("POST /admin/api/auth/logout", a.handleLogout)
	mux.HandleFunc("GET /admin/api/auth/me", a.handleMe)
	mux.HandleFunc("GET /admin/api/overview", a.auth(a.handleOverview))
	mux.HandleFunc("GET /admin/api/docs", a.auth(a.handleDocs))
	mux.HandleFunc("GET /admin/api/accounts", a.auth(a.handleListAccounts))
	mux.HandleFunc("POST /admin/api/accounts", a.auth(a.handleAddAccount))
	mux.HandleFunc("POST /admin/api/accounts/gemini", a.auth(a.handleAddGemini))
	mux.HandleFunc("POST /admin/api/accounts/aistudio", a.auth(a.handleAddAIStudio))
	mux.HandleFunc("PATCH /admin/api/accounts/{id}", a.auth(a.handlePatchAccount))
	mux.HandleFunc("PUT /admin/api/accounts/{id}/credentials", a.auth(a.handleUpdateCredentials))
	mux.HandleFunc("POST /admin/api/accounts/{id}/check", a.auth(a.handleCheckAccount))
	mux.HandleFunc("DELETE /admin/api/accounts/{id}", a.auth(a.handleDeleteAccount))
	mux.HandleFunc("GET /admin/api/keys", a.auth(a.handleListKeys))
	mux.HandleFunc("POST /admin/api/keys", a.auth(a.handleCreateKey))
	mux.HandleFunc("PATCH /admin/api/keys/{id}", a.auth(a.handlePatchKey))
	mux.HandleFunc("DELETE /admin/api/keys/{id}", a.auth(a.handleDeleteKey))
	mux.HandleFunc("GET /admin/api/usage", a.auth(a.handleUsage))
	mux.HandleFunc("GET /admin/api/usage/records", a.auth(a.handleUsageRecords))
	mux.HandleFunc("GET /admin/api/usage/timeseries", a.auth(a.handleUsageTimeseries))
	mux.HandleFunc("GET /admin/api/usage/export.csv", a.auth(a.handleUsageExport))
	mux.HandleFunc("POST /admin/api/keys/{id}/regenerate", a.auth(a.handleRegenerateKey))
	mux.HandleFunc("GET /admin/api/models", a.auth(a.handleModels))
	mux.HandleFunc("GET /admin/api/version", a.auth(a.handleVersion))
	mux.HandleFunc("GET /admin/api/multipliers", a.auth(a.handleListMultipliers))
	mux.HandleFunc("PUT /admin/api/multipliers", a.auth(a.handlePutMultiplier))
	mux.HandleFunc("DELETE /admin/api/multipliers/{model...}", a.auth(a.handleDeleteMultiplier))
	mux.HandleFunc("GET /admin/api/status", a.auth(a.handleStatus))
	mux.HandleFunc("GET /admin/api/upgrade", a.auth(a.handleUpgradeStatus))
	mux.HandleFunc("POST /admin/api/upgrade/check", a.auth(a.handleUpgradeCheck))
	mux.HandleFunc("POST /admin/api/upgrade", a.auth(a.handleUpgradeStart))

	// 平台：用户、余额、兑换码、账单流水、设置
	mux.HandleFunc("GET /admin/api/users", a.adminAuth(a.handleAdminUsers))
	mux.HandleFunc("POST /admin/api/users", a.adminAuth(a.handleAdminCreateUser))
	mux.HandleFunc("GET /admin/api/users/{id}", a.adminAuth(a.handleAdminUser))
	mux.HandleFunc("PATCH /admin/api/users/{id}", a.adminAuth(a.handleAdminPatchUser))
	mux.HandleFunc("DELETE /admin/api/users/{id}", a.adminAuth(a.handleAdminDeleteUser))
	mux.HandleFunc("POST /admin/api/users/{id}/password", a.adminAuth(a.handleAdminUserPassword))
	mux.HandleFunc("POST /admin/api/users/{id}/balance", a.adminAuth(a.handleAdminAdjustBalance))
	mux.HandleFunc("GET /admin/api/ledger", a.adminAuth(a.handleAdminLedger))
	mux.HandleFunc("POST /admin/api/usage/records/{id}/refund", a.adminAuth(a.handleAdminRefund))
	mux.HandleFunc("GET /admin/api/redeem-codes", a.adminAuth(a.handleAdminCodes))
	mux.HandleFunc("POST /admin/api/redeem-codes", a.adminAuth(a.handleAdminCreateCodes))
	mux.HandleFunc("GET /admin/api/redeem-codes/export.csv", a.adminAuth(a.handleAdminExportCodes))
	mux.HandleFunc("PATCH /admin/api/redeem-codes/{id}", a.adminAuth(a.handleAdminPatchCode))
	mux.HandleFunc("DELETE /admin/api/redeem-codes/{id}", a.adminAuth(a.handleAdminDeleteCode))
	mux.HandleFunc("POST /admin/api/redeem-codes/batches/{batch}/disable", a.adminAuth(a.handleAdminDisableBatch))
	mux.HandleFunc("GET /admin/api/settings", a.adminAuth(a.handleAdminSettings))
	mux.HandleFunc("PUT /admin/api/settings", a.adminAuth(a.handleAdminPutSettings))

	// 公开与平台用户接口（同一套 JWT + Redis 会话）
	mux.HandleFunc("GET /api/public/config", a.handlePublicConfig)
	mux.HandleFunc("POST /api/auth/register", a.handleRegister)
	mux.HandleFunc("POST /api/auth/login", a.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", a.handleLogout)
	mux.HandleFunc("GET /api/auth/me", a.handleMe)
	mux.HandleFunc("GET /api/user/overview", a.userAuth(a.handleUserOverview))
	mux.HandleFunc("GET /api/user/keys", a.userAuth(a.handleUserKeys))
	mux.HandleFunc("POST /api/user/keys", a.userAuth(a.handleUserCreateKey))
	mux.HandleFunc("PATCH /api/user/keys/{id}", a.userAuth(a.handleUserPatchKey))
	mux.HandleFunc("DELETE /api/user/keys/{id}", a.userAuth(a.handleUserDeleteKey))
	mux.HandleFunc("POST /api/user/keys/{id}/regenerate", a.userAuth(a.handleUserRegenerateKey))
	mux.HandleFunc("GET /api/user/usage/records", a.userAuth(a.handleUserUsageRecords))
	mux.HandleFunc("GET /api/user/usage/timeseries", a.userAuth(a.handleUserUsageTimeseries))
	mux.HandleFunc("GET /api/user/ledger", a.userAuth(a.handleUserLedger))
	mux.HandleFunc("GET /api/user/redeem", a.userAuth(a.handleUserRedeemHistory))
	mux.HandleFunc("POST /api/user/redeem", a.userAuth(a.handleUserRedeem))
	mux.HandleFunc("PATCH /api/user/profile", a.userAuth(a.handleUserProfile))
	mux.HandleFunc("POST /api/user/password", a.userAuth(a.handleUserPassword))
	mux.HandleFunc("GET /api/user/models", a.userAuth(a.handleUserModels))
}

// handleDocs 返回管理 API 的 OpenAPI 3.1 文档。
// 文档与管理接口一起鉴权，避免在未登录时暴露部署细节；前端管理台会直接消费该文档。
func (a *API) handleDocs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, openAPISpec())
}

// openAPISpec 使用标准 OpenAPI 结构，避免引入额外的文档生成依赖。
// startedAt 进程启动时间（总览显示运行时长）。
var startedAt = time.Now()

var (
	tokenLimitSchema    = map[string]any{"type": "integer", "minimum": 0, "description": "Token 额度（按倍率计费后的 Token），0 表示不限。用尽后网关返回 429 insufficient_quota"}
	expiresSchema       = map[string]any{"type": "integer", "minimum": 0, "description": "过期时间（Unix 秒），0 表示永不过期。过期后网关返回 401 key_expired"}
	allowedModelsSchema = map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "模型白名单，空数组表示允许全部；支持末尾 * 前缀匹配，如 gemini-3.5-*。不允许时网关返回 403 model_not_allowed"}
	rpmSchema           = map[string]any{"type": "integer", "minimum": 0, "description": "每分钟请求数上限，0 表示只受全局限流约束。超限返回 429 rpm_limit_exceeded"}
	keyMultiplierSchema = map[string]any{"type": "number", "minimum": 0, "maximum": store.MaxMultiplier, "description": "Key 倍率（分组默认倍率），与模型倍率相乘，默认 1"}
)

func usageParams(records bool) []any {
	params := []any{
		map[string]any{"name": "days", "in": "query", "description": "统计天数，1-90，默认 7", "schema": map[string]any{"type": "integer", "default": 7, "minimum": 1, "maximum": 90}},
		map[string]any{"name": "key_id", "in": "query", "description": "只看某个 Key（breakdown/records）", "schema": map[string]any{"type": "integer"}},
		map[string]any{"name": "model", "in": "query", "description": "只看某个模型（breakdown/records）", "schema": map[string]any{"type": "string"}},
	}
	if records {
		params = append(params,
			map[string]any{"name": "limit", "in": "query", "schema": map[string]any{"type": "integer", "default": 100, "maximum": 500}},
			map[string]any{"name": "offset", "in": "query", "schema": map[string]any{"type": "integer", "default": 0}})
	}
	return params
}

func openAPISpec() map[string]any {
	jsonBody := map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object"}}}
	response := func(description string) map[string]any {
		return map[string]any{"description": description, "content": jsonBody}
	}
	body := func(schema string) map[string]any {
		return map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/" + schema}}}}
	}
	return addPlatformSpec(map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "web2api 管理 API",
			"version":     "1.0.0",
			"description": "号池、API Key、用量、用户余额、兑换码和引擎状态管理接口，以及 /api/* 用户控制台接口。管理接口使用 JWT 验证及 Redis 会话；初始化和登录接口无需登录。",
		},
		"servers":  []any{map[string]any{"url": "/", "description": "当前 web2api 服务"}},
		"security": []any{map[string]any{"AdminJWT": []any{}}},
		"tags": []any{
			map[string]any{"name": "概览", "description": "运行状态和管理台数据"},
			map[string]any{"name": "账号", "description": "Gemini 与 AI Studio 号池"},
			map[string]any{"name": "API Key", "description": "客户端 API Key 的生命周期管理"},
			map[string]any{"name": "用量", "description": "按 Key、引擎和模型聚合的请求用量"},
		},
		"paths": map[string]any{
			"/admin/api/setup": map[string]any{
				"get":  map[string]any{"summary": "查询初始化状态", "security": []any{}, "responses": map[string]any{"200": response("initialized")}},
				"post": map[string]any{"summary": "首次初始化管理员和 Redis", "security": []any{}, "requestBody": body("SetupInput"), "responses": map[string]any{"201": response("初始化成功"), "400": response("参数或 Redis 连接无效"), "409": response("已经初始化")}},
			},
			"/admin/api/auth/login":  map[string]any{"post": map[string]any{"summary": "邮箱密码登录", "security": []any{}, "requestBody": body("LoginInput"), "responses": map[string]any{"200": response("access_token、token_type、expires_in、user"), "401": response("邮箱或密码错误"), "429": response("登录尝试过多"), "503": response("Redis 不可用")}}},
			"/admin/api/auth/logout": map[string]any{"post": map[string]any{"summary": "退出并撤销当前 JWT", "responses": map[string]any{"200": response("已退出"), "401": response("登录已失效")}}},
			"/admin/api/auth/me":     map[string]any{"get": map[string]any{"summary": "获取管理员邮箱与昵称", "responses": map[string]any{"200": response("user"), "401": response("登录已失效")}}},
			"/admin/api/overview":    map[string]any{"get": map[string]any{"tags": []string{"概览"}, "summary": "获取总览统计", "responses": map[string]any{"200": response("总览统计"), "401": response("JWT 无效或已过期")}}},
			"/admin/api/docs":        map[string]any{"get": map[string]any{"tags": []string{"概览"}, "summary": "获取 OpenAPI 文档", "responses": map[string]any{"200": response("OpenAPI 3.1 文档")}}},
			"/admin/api/status":      map[string]any{"get": map[string]any{"tags": []string{"概览"}, "summary": "获取引擎详细状态", "responses": map[string]any{"200": response("引擎状态")}}},
			"/admin/api/upgrade": map[string]any{
				"get":  map[string]any{"tags": []string{"概览"}, "summary": "获取镜像升级状态", "responses": map[string]any{"200": response("当前镜像、目标镜像和升级任务状态")}},
				"post": map[string]any{"tags": []string{"概览"}, "summary": "升级到已经检查的最新镜像", "description": "仅支持已启用升级配置的 Docker 部署；重启期间短暂断连，失败自动恢复旧容器。", "responses": map[string]any{"202": response("升级任务已启动"), "409": response("任务正在运行或没有可用更新"), "503": response("部署不支持升级或 Docker 不可用")}},
			},
			"/admin/api/upgrade/check": map[string]any{"post": map[string]any{"tags": []string{"概览"}, "summary": "检查并拉取最新镜像", "responses": map[string]any{"202": response("镜像检查已启动"), "409": response("已有任务运行"), "503": response("部署不支持升级")}}},
			"/admin/api/accounts": map[string]any{
				"get":  map[string]any{"tags": []string{"账号"}, "summary": "列出账号", "responses": map[string]any{"200": response("账号列表")}},
				"post": map[string]any{"tags": []string{"账号"}, "summary": "按引擎2协议添加账号", "description": "所有账号统一提交 storage_state；engine=a 时从其中提取 Gemini Cookie。", "requestBody": body("AccountInput"), "responses": map[string]any{"200": response("账号已创建"), "400": response("参数错误")}},
			},
			"/admin/api/accounts/gemini":   map[string]any{"post": map[string]any{"tags": []string{"账号"}, "summary": "添加 Gemini 账号", "requestBody": body("GeminiAccountInput"), "responses": map[string]any{"200": response("账号已创建"), "400": response("参数错误")}}},
			"/admin/api/accounts/aistudio": map[string]any{"post": map[string]any{"tags": []string{"账号"}, "summary": "添加 AI Studio 账号", "requestBody": body("AIStudioAccountInput"), "responses": map[string]any{"200": response("账号已创建"), "400": response("参数错误")}}},
			"/admin/api/accounts/{id}": map[string]any{
				"parameters": []any{map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "integer", "format": "int64"}}},
				"patch":      map[string]any{"tags": []string{"账号"}, "summary": "启用或停用账号", "requestBody": body("EnabledInput"), "responses": map[string]any{"200": response("更新成功"), "400": response("参数错误")}},
				"delete":     map[string]any{"tags": []string{"账号"}, "summary": "删除账号", "responses": map[string]any{"200": response("删除成功"), "404": response("账号不存在")}},
			},
			"/admin/api/accounts/{id}/credentials": map[string]any{"put": map[string]any{"tags": []string{"账号"}, "summary": "更新 Gemini Cookie", "requestBody": body("GeminiCredentialsInput"), "responses": map[string]any{"200": response("更新成功"), "400": response("参数错误")}}},
			"/admin/api/accounts/{id}/check":       map[string]any{"post": map[string]any{"tags": []string{"账号"}, "summary": "触发账号健康检测", "responses": map[string]any{"200": response("检测已触发")}}},
			"/admin/api/keys": map[string]any{
				"get":  map[string]any{"tags": []string{"API Key"}, "summary": "列出 API Key", "responses": map[string]any{"200": response("Key 列表")}},
				"post": map[string]any{"tags": []string{"API Key"}, "summary": "创建 API Key", "requestBody": body("KeyInput"), "responses": map[string]any{"200": response("Key 已创建")}},
			},
			"/admin/api/keys/{id}": map[string]any{
				"parameters": []any{map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "integer", "format": "int64"}}},
				"patch":      map[string]any{"tags": []string{"API Key"}, "summary": "修改 API Key（启停、备注、Token 额度、倍率、重置已用）", "description": "所有字段可选，至少提供一个。返回 {ok, key}。", "requestBody": body("KeyPatchInput"), "responses": map[string]any{"200": response("更新成功"), "400": response("参数错误"), "404": response("Key 不存在")}},
				"delete":     map[string]any{"tags": []string{"API Key"}, "summary": "删除 API Key", "responses": map[string]any{"200": response("删除成功")}},
			},
			"/admin/api/usage":                map[string]any{"get": map[string]any{"tags": []string{"用量"}, "summary": "查询用量聚合", "description": "usage 为旧的按 Key/引擎/模型聚合（兼容）；breakdown 按 Key×模型汇总逐请求明细，含 total_tokens、charged_tokens、estimated_requests；totals 为合计。", "parameters": usageParams(false), "responses": map[string]any{"200": response("days、usage、breakdown、totals")}}},
			"/admin/api/usage/records":        map[string]any{"get": map[string]any{"tags": []string{"用量"}, "summary": "逐请求用量明细", "description": "每条含 prompt/completion/total_tokens、estimated（true=本地估算，上游未返回用量）、model_multiplier、key_multiplier、multiplier、charged_tokens。Key 以 key_masked 脱敏显示。", "parameters": usageParams(true), "responses": map[string]any{"200": response("days、total、limit、offset、records")}}},
			"/admin/api/keys/{id}/regenerate": map[string]any{"post": map[string]any{"tags": []string{"API Key"}, "summary": "重新生成密钥", "description": "换发新的 sk- 密钥，旧密钥立即失效；额度、倍率、白名单等设置保留。", "parameters": []any{map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "integer", "format": "int64"}}}, "responses": map[string]any{"200": response("key"), "404": response("Key 不存在")}}},
			"/admin/api/usage/timeseries":     map[string]any{"get": map[string]any{"tags": []string{"用量"}, "summary": "用量时间序列", "description": "days=1 按小时，其余按本地自然日分桶；空桶补零。每点含 requests、success_requests、total_tokens、charged_tokens、avg_latency_ms。", "parameters": usageParams(false), "responses": map[string]any{"200": response("bucket、points")}}},
			"/admin/api/usage/export.csv":     map[string]any{"get": map[string]any{"tags": []string{"用量"}, "summary": "导出逐请求明细 CSV", "description": "UTF-8（带 BOM），最多 100000 行，Key 脱敏。", "parameters": usageParams(false), "responses": map[string]any{"200": map[string]any{"description": "CSV 文件", "content": map[string]any{"text/csv": map[string]any{}}}}}},
			"/admin/api/models":               map[string]any{"get": map[string]any{"tags": []string{"概览"}, "summary": "模型目录（含生效倍率）", "responses": map[string]any{"200": response("models")}}},
			"/admin/api/version":              map[string]any{"get": map[string]any{"tags": []string{"概览"}, "summary": "版本信息", "responses": map[string]any{"200": response("version、commit、build_time、go_version")}}},
			"/admin/api/multipliers": map[string]any{
				"get": map[string]any{"tags": []string{"用量"}, "summary": "列出模型倍率", "description": "charged_tokens = ceil(total_tokens × 模型倍率 × Key 倍率 × 用户倍率)，用户倍率只对用户 Key 生效。model=\"*\" 为未配置模型的默认倍率；均未配置时为 1。", "responses": map[string]any{"200": response("multipliers、default_multiplier、models、formula")}},
				"put": map[string]any{"tags": []string{"用量"}, "summary": "设置模型倍率", "requestBody": body("MultiplierInput"), "responses": map[string]any{"200": response("已保存"), "400": response("参数错误")}},
			},
			"/admin/api/multipliers/{model}": map[string]any{"delete": map[string]any{"tags": []string{"用量"}, "summary": "删除模型倍率（恢复默认）", "parameters": []any{map[string]any{"name": "model", "in": "path", "required": true, "schema": map[string]any{"type": "string"}}}, "responses": map[string]any{"200": response("已删除"), "404": response("未配置")}}},
		},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"AdminJWT": map[string]any{"type": "http", "scheme": "bearer", "bearerFormat": "JWT", "description": "邮箱密码登录签发的 JWT，8 小时有效；退出时撤销 Redis 会话"},
			},
			"schemas": map[string]any{
				"SetupInput":             map[string]any{"type": "object", "required": []string{"email", "password", "nickname", "redis_url"}, "properties": map[string]any{"email": map[string]any{"type": "string", "format": "email"}, "password": map[string]any{"type": "string", "minLength": 8, "description": "最多 72 字节", "writeOnly": true}, "nickname": map[string]any{"type": "string", "minLength": 1, "maxLength": 64}, "redis_url": map[string]any{"type": "string", "description": "redis://[user:password@]host:port/db，支持 rediss:// TLS", "writeOnly": true}}},
				"LoginInput":             map[string]any{"type": "object", "required": []string{"email", "password"}, "properties": map[string]any{"email": map[string]any{"type": "string", "format": "email"}, "password": map[string]any{"type": "string", "writeOnly": true}}},
				"EnabledInput":           map[string]any{"type": "object", "required": []string{"enabled"}, "properties": map[string]any{"enabled": map[string]any{"type": "boolean", "description": "是否允许调度或调用"}}},
				"KeyInput":               map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string", "description": "备注名，可留空"}, "token_limit": tokenLimitSchema, "multiplier": keyMultiplierSchema, "expires_at": expiresSchema, "allowed_models": allowedModelsSchema, "rpm_limit": rpmSchema}},
				"KeyPatchInput":          map[string]any{"type": "object", "properties": map[string]any{"enabled": map[string]any{"type": "boolean"}, "name": map[string]any{"type": "string"}, "token_limit": tokenLimitSchema, "multiplier": keyMultiplierSchema, "reset_usage": map[string]any{"type": "boolean", "description": "true 时把 tokens_used 归零（保留用量明细）"}, "expires_at": expiresSchema, "allowed_models": allowedModelsSchema, "rpm_limit": rpmSchema}},
				"MultiplierInput":        map[string]any{"type": "object", "required": []string{"model", "multiplier"}, "properties": map[string]any{"model": map[string]any{"type": "string", "description": "模型 ID；\"*\" 为默认倍率"}, "multiplier": map[string]any{"type": "number", "minimum": 0, "maximum": store.MaxMultiplier}}},
				"GeminiAccountInput":     map[string]any{"type": "object", "required": []string{"label", "psid", "psidts"}, "properties": map[string]any{"label": map[string]any{"type": "string"}, "psid": map[string]any{"type": "string", "description": "__Secure-1PSID Cookie"}, "psidts": map[string]any{"type": "string", "description": "__Secure-1PSIDTS Cookie"}}},
				"GeminiCredentialsInput": map[string]any{"type": "object", "required": []string{"psid", "psidts"}, "properties": map[string]any{"psid": map[string]any{"type": "string"}, "psidts": map[string]any{"type": "string"}}},
				"AIStudioAccountInput":   map[string]any{"type": "object", "required": []string{"email", "storage_state"}, "properties": map[string]any{"email": map[string]any{"type": "string", "format": "email"}, "storage_state": map[string]any{"type": "string", "description": "storage-state.json 原文"}, "locale": map[string]any{"type": "string"}, "timezone": map[string]any{"type": "string"}, "proxy": map[string]any{"type": "string"}}},
				"AccountInput":           map[string]any{"type": "object", "required": []string{"engine", "storage_state"}, "properties": map[string]any{"engine": map[string]any{"type": "string", "enum": []string{"a", "b"}, "description": "a=Gemini，b=AI Studio"}, "label": map[string]any{"type": "string"}, "email": map[string]any{"type": "string", "format": "email"}, "storage_state": map[string]any{"type": "string", "description": "引擎2协议的 storage-state.json 原文"}, "locale": map[string]any{"type": "string"}, "timezone": map[string]any{"type": "string"}, "proxy": map[string]any{"type": "string"}}},
			},
		},
	})
}

// auth 管理鉴权中间件。
func (a *API) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, _, code, err := a.authenticate(r); err != nil {
			writeJSON(w, code, map[string]any{"error": err.Error()})
			return
		}
		next(w, r)
	}
}

func bearer(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return errors.New("请求体为空")
	}
	return json.Unmarshal(body, v)
}

func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

// ==================== 总览 ====================

// handleOverview 管理台首页数据。
func (a *API) handleOverview(w http.ResponseWriter, r *http.Request) {
	accounts := a.mgr.AccountOverview()
	keys, _ := a.st.ListKeys()
	since := time.Now().AddDate(0, 0, -1).Unix()
	usage, _ := a.st.UsageSummary(since)

	var readyA, readyB, totalReqs int64
	for _, acc := range accounts {
		switch acc.Engine {
		case "a":
			if acc.Live.Ready {
				readyA++
			}
		case "b":
			if acc.Live.Ready {
				readyB++
			}
		}
	}
	for _, u := range usage {
		totalReqs += u.Requests
	}
	var keysLimited, keysExhausted int
	for _, k := range keys {
		if k.TokenLimit > 0 {
			keysLimited++
		}
		if k.QuotaExhausted {
			keysExhausted++
		}
	}
	var tokens24h, charged24h int64
	if breakdown, err := a.st.UsageBreakdown(store.UsageFilter{Since: since}); err == nil {
		for _, row := range breakdown {
			tokens24h += row.TotalTokens
			charged24h += row.ChargedTokens
		}
	}
	platform, _ := a.st.GetPlatformTotals()
	writeJSON(w, http.StatusOK, map[string]any{
		"platform":           platform,
		"accounts_total":     len(accounts),
		"accounts_ready":     readyA + readyB,
		"engine_a_ready":     readyA,
		"engine_b_ready":     readyB,
		"keys_total":         len(keys),
		"requests_24h":       totalReqs,
		"keys_limited":       keysLimited,
		"keys_exhausted":     keysExhausted,
		"tokens_24h":         tokens24h,
		"charged_tokens_24h": charged24h,
		"version":            version.Get(),
		"uptime_seconds":     int64(time.Since(startedAt).Seconds()),
	})
}

// handleStatus 引擎详细状态（/v1/accounts 的管理版）。
func (a *API) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.mgr.Status())
}

// ==================== 账号 ====================

// handleAddAccount 是统一账号添加入口。旧的 /gemini 和 /aistudio 接口
// 仍可使用，但新的协议只接受引擎2的 storage-state 格式。
func (a *API) handleAddAccount(w http.ResponseWriter, r *http.Request) {
	var req provider.AccountInput
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	acc, err := a.mgr.AddAccountFromStorageState(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"account": acc})
}

func (a *API) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"accounts": a.mgr.AccountOverview()})
}

// handleAddGemini 添加 Gemini 网页号。
func (a *API) handleAddGemini(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Label  string `json:"label"`
		PSID   string `json:"psid"`
		PSIDTS string `json:"psidts"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.Label) == "" || strings.TrimSpace(req.PSID) == "" || strings.TrimSpace(req.PSIDTS) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "label、psid、psidts 均不能为空"})
		return
	}
	// 兼容旧字段：先转换成引擎2 storage-state，再走统一入口。
	state, _ := json.Marshal(map[string]any{"cookies": []map[string]any{
		{"name": "__Secure-1PSID", "value": req.PSID, "domain": ".google.com", "path": "/", "secure": true},
		{"name": "__Secure-1PSIDTS", "value": req.PSIDTS, "domain": ".google.com", "path": "/", "secure": true},
	}})
	acc, err := a.mgr.AddAccountFromStorageState(provider.AccountInput{
		Engine: "a", Label: req.Label, StorageState: string(state),
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"account": acc})
}

// handleAddAIStudio 添加 AI Studio 号。
func (a *API) handleAddAIStudio(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email        string `json:"email"`
		StorageState string `json:"storage_state"` // storage-state JSON 文本
		Locale       string `json:"locale"`
		Timezone     string `json:"timezone"`
		Proxy        string `json:"proxy"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.Email) == "" || strings.TrimSpace(req.StorageState) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "email、storage_state 均不能为空"})
		return
	}
	acc, err := a.mgr.AddAccountFromStorageState(provider.AccountInput{
		Engine: "b", Email: req.Email, StorageState: req.StorageState,
		Locale: req.Locale, Timezone: req.Timezone, Proxy: req.Proxy,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"account": acc})
}

// handlePatchAccount 启停账号。
func (a *API) handlePatchAccount(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效 ID"})
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if req.Enabled == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "仅支持 enabled 字段"})
		return
	}
	if err := a.mgr.SetAccountEnabled(id, *req.Enabled); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleUpdateCredentials 更新 Gemini 号 Cookie。
func (a *API) handleUpdateCredentials(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效 ID"})
		return
	}
	var req struct {
		PSID   string `json:"psid"`
		PSIDTS string `json:"psidts"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.PSID) == "" || strings.TrimSpace(req.PSIDTS) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "psid、psidts 均不能为空"})
		return
	}
	if err := a.mgr.UpdateGeminiCredentials(id, req.PSID, req.PSIDTS); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleCheckAccount 触发健康检查。
func (a *API) handleCheckAccount(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效 ID"})
		return
	}
	if err := a.mgr.CheckAccount(r.Context(), id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleDeleteAccount 删除账号。
func (a *API) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效 ID"})
		return
	}
	if err := a.mgr.RemoveAccount(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ==================== API Key ====================

func (a *API) handleListKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := a.st.ListKeys()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	// 附加 24h 用量
	since := time.Now().AddDate(0, 0, -1).Unix()
	usage, _ := a.st.UsageSummary(since)
	usageByKey := map[string]int64{}
	for _, u := range usage {
		usageByKey[u.APIKey] += u.Requests
	}
	charged24h := map[int64]int64{}
	if breakdown, err := a.st.UsageBreakdown(store.UsageFilter{Since: since}); err == nil {
		for _, row := range breakdown {
			charged24h[row.KeyID] += row.ChargedTokens
		}
	}
	type keyView struct {
		store.APIKey
		Requests24h      int64  `json:"requests_24h"`
		ChargedTokens24h int64  `json:"charged_tokens_24h"`
		OwnerEmail       string `json:"owner_email,omitempty"`
	}
	emails, _ := a.st.UserEmails()
	out := make([]keyView, 0, len(keys))
	for _, k := range keys {
		out = append(out, keyView{APIKey: k, Requests24h: usageByKey[k.Key], ChargedTokens24h: charged24h[k.ID], OwnerEmail: emails[k.UserID]})
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": out})
}

func (a *API) handleCreateKey(w http.ResponseWriter, r *http.Request) {
	var req KeyOptions
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if err := req.validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	name := ""
	if req.Name != nil {
		name = *req.Name
	}
	key, err := a.st.CreateKey(name)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	// 额度与倍率可在创建时一并设置（name 已写入，不再重复）。
	opts := req
	opts.Name, opts.ResetUsage = nil, false
	if update := opts.update(); !update.Empty() {
		if key, err = a.st.UpdateKey(key.ID, update); err != nil {
			storeError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"key": key})
}

func (a *API) handlePatchKey(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效 ID"})
		return
	}
	var req KeyOptions
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	update := req.update()
	if update.Empty() {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请至少提供 enabled、name、token_limit、multiplier 或 reset_usage 之一"})
		return
	}
	if err := req.validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	key, err := a.st.UpdateKey(id, update)
	if err != nil {
		storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "key": key})
}

func (a *API) handleDeleteKey(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效 ID"})
		return
	}
	if err := a.st.DeleteKey(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ==================== 用量 ====================

// handleUsage 用量统计。?days=7（默认 7 天，最多 90 天）。
func (a *API) handleUsage(w http.ResponseWriter, r *http.Request) {
	filter, days := usageFilter(r)
	usage, err := a.st.UsageSummary(filter.Since)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	// breakdown 来自逐请求明细表（含 total/charged/estimated），usage 保持旧聚合口径。
	breakdown, err := a.st.UsageBreakdown(filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	var totals store.UsageBreakdownRow
	for _, row := range breakdown {
		totals.Requests += row.Requests
		totals.SuccessRequests += row.SuccessRequests
		totals.PromptTokens += row.PromptTokens
		totals.CompletionTokens += row.CompletionTokens
		totals.TotalTokens += row.TotalTokens
		totals.ChargedTokens += row.ChargedTokens
		totals.EstimatedRequests += row.EstimatedRequests
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"days":      days,
		"usage":     usage,
		"breakdown": breakdown,
		"totals": map[string]any{
			"requests": totals.Requests, "success_requests": totals.SuccessRequests,
			"prompt_tokens": totals.PromptTokens, "completion_tokens": totals.CompletionTokens,
			"total_tokens": totals.TotalTokens, "charged_tokens": totals.ChargedTokens,
			"estimated_requests": totals.EstimatedRequests,
		},
	})
}
