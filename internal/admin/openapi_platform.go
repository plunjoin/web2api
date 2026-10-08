package admin

// 平台（用户、余额、兑换码、用户控制台）相关的 OpenAPI 描述，合并进 openAPISpec。

func addPlatformSpec(spec map[string]any) map[string]any {
	jsonBody := map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object"}}}
	response := func(description string) map[string]any {
		return map[string]any{"description": description, "content": jsonBody}
	}
	body := func(schema string) map[string]any {
		return map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/" + schema}}}}
	}
	idParam := []any{map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "integer", "format": "int64"}}}
	query := func(name, description string, typ string) map[string]any {
		return map[string]any{"name": name, "in": "query", "description": description, "schema": map[string]any{"type": typ}}
	}
	page := []any{query("limit", "每页条数", "integer"), query("offset", "偏移", "integer")}
	with := func(extra ...map[string]any) []any {
		out := make([]any, 0, len(extra)+len(page))
		for _, e := range extra {
			out = append(out, e)
		}
		return append(out, page...)
	}
	ledgerParams := with(query("user_id", "按用户筛选", "integer"), query("kind", "signup/redeem/adjust/usage/refund", "string"), query("q", "备注或邮箱关键词", "string"), query("days", "最近天数", "integer"))
	str := map[string]any{"type": "string"}
	boolean := map[string]any{"type": "boolean"}
	integer := map[string]any{"type": "integer", "format": "int64"}
	multiplier := map[string]any{"type": "number", "minimum": 0, "description": "用户倍率，参与 ceil(total × 模型 × Key × 用户)"}
	role := map[string]any{"type": "string", "enum": []string{"user", "admin"}}
	public := []any{}
	user := []string{"用户控制台"}

	paths := map[string]any{
		"/admin/api/users": map[string]any{
			"get":  map[string]any{"tags": []string{"用户"}, "summary": "用户列表", "parameters": with(query("q", "邮箱或昵称", "string"), query("role", "user/admin", "string"), query("status", "enabled/disabled", "string")), "responses": map[string]any{"200": response("users、total")}},
			"post": map[string]any{"tags": []string{"用户"}, "summary": "新建用户", "description": "balance>0 时写一条 adjust 流水（备注「创建账号时的初始余额」）；note 为用户备注。multiplier 省略时使用平台默认用户倍率。", "requestBody": body("UserCreateInput"), "responses": map[string]any{"201": response("user"), "400": response("参数错误"), "409": response("邮箱已被使用")}},
		},
		"/admin/api/users/{id}": map[string]any{
			"parameters": idParam,
			"get":        map[string]any{"tags": []string{"用户"}, "summary": "用户详情", "description": "含 keys、最近流水、total_credit/total_debit（全部流水的入账与出账合计）。", "responses": map[string]any{"200": response("user、keys、ledger、week、total_credit、total_debit"), "404": response("用户不存在")}},
			"patch":      map[string]any{"tags": []string{"用户"}, "summary": "修改昵称、角色、启停、用户倍率、备注", "requestBody": body("UserPatchInput"), "responses": map[string]any{"200": response("user"), "400": response("参数错误，或停用/降级自己")}},
			"delete":     map[string]any{"tags": []string{"用户"}, "summary": "删除用户", "description": "同时删除其 Key 与余额流水；历史用量记录保留。", "responses": map[string]any{"200": response("已删除"), "400": response("不能删除自己"), "404": response("用户不存在")}},
		},
		"/admin/api/users/{id}/password":       map[string]any{"post": map[string]any{"tags": []string{"用户"}, "summary": "重置用户密码", "description": "同时使该用户所有会话失效。", "parameters": idParam, "requestBody": body("PasswordInput"), "responses": map[string]any{"200": response("已重置"), "400": response("密码不符合要求")}}},
		"/admin/api/users/{id}/balance":        map[string]any{"post": map[string]any{"tags": []string{"用户"}, "summary": "调整余额", "description": "amount 为正数加款、负数扣款；note 必填。扣款后余额不能低于 0。写 adjust 流水并返回变动前后余额。", "parameters": idParam, "requestBody": body("BalanceAdjustInput"), "responses": map[string]any{"200": response("entry（含 balance_before/balance_after）、balance"), "400": response("缺少备注、金额为 0 或扣款后余额为负")}}},
		"/admin/api/ledger":                    map[string]any{"get": map[string]any{"tags": []string{"用户"}, "summary": "余额流水", "parameters": ledgerParams, "responses": map[string]any{"200": response("entries（含 balance_before/balance_after）、total、sums")}}},
		"/admin/api/usage/records/{id}/refund": map[string]any{"post": map[string]any{"tags": []string{"用户"}, "summary": "对一次用户请求退款", "description": "按该请求实际扣费金额退回余额，写 refund 流水；每条记录只能退一次。", "parameters": idParam, "requestBody": body("NoteInput"), "responses": map[string]any{"200": response("entry"), "400": response("该请求不属于用户或未扣费"), "404": response("记录不存在"), "409": response("已退款")}}},
		"/admin/api/redeem-codes": map[string]any{
			"get":  map[string]any{"tags": []string{"兑换码"}, "summary": "兑换码列表", "parameters": with(query("status", "unused/redeemed/disabled/expired", "string"), query("batch", "批次", "string"), query("q", "码、备注或兑换人", "string"), query("user_id", "兑换人", "integer")), "responses": map[string]any{"200": response("codes、total、stats")}},
			"post": map[string]any{"tags": []string{"兑换码"}, "summary": "批量生成兑换码", "description": "完整兑换码只在此响应中批量返回一次以便复制或下载；列表与导出同样可见，请妥善保管。", "requestBody": body("RedeemCodeCreateInput"), "responses": map[string]any{"201": response("batch、codes"), "400": response("参数错误")}},
		},
		"/admin/api/redeem-codes/export.csv": map[string]any{"get": map[string]any{"tags": []string{"兑换码"}, "summary": "导出兑换码 CSV", "parameters": []any{query("status", "状态", "string"), query("batch", "批次", "string"), query("q", "关键词", "string")}, "responses": map[string]any{"200": map[string]any{"description": "CSV 文件", "content": map[string]any{"text/csv": map[string]any{}}}}}},
		"/admin/api/redeem-codes/{id}": map[string]any{
			"parameters": idParam,
			"patch":      map[string]any{"tags": []string{"兑换码"}, "summary": "启用或停用兑换码", "requestBody": body("EnabledInput"), "responses": map[string]any{"200": response("code")}},
			"delete":     map[string]any{"tags": []string{"兑换码"}, "summary": "删除未兑换的兑换码", "responses": map[string]any{"200": response("已删除"), "409": response("已兑换的码不能删除")}},
		},
		"/admin/api/redeem-codes/batches/{batch}/disable": map[string]any{"post": map[string]any{"tags": []string{"兑换码"}, "summary": "停用整个批次中未兑换的码", "parameters": []any{map[string]any{"name": "batch", "in": "path", "required": true, "schema": str}}, "responses": map[string]any{"200": response("disabled（停用数量）")}}},
		"/admin/api/settings": map[string]any{
			"get": map[string]any{"tags": []string{"用户"}, "summary": "平台设置", "responses": map[string]any{"200": response("settings")}},
			"put": map[string]any{"tags": []string{"用户"}, "summary": "修改平台设置", "requestBody": body("PlatformSettingsInput"), "responses": map[string]any{"200": response("settings"), "400": response("参数错误")}},
		},
		"/api/public/config": map[string]any{"get": map[string]any{"tags": user, "summary": "站点名、是否开放注册、注册赠送", "security": public, "responses": map[string]any{"200": response("initialized、registration_open、signup_bonus、site_name、version")}}},
		"/api/auth/register": map[string]any{"post": map[string]any{"tags": user, "summary": "注册", "security": public, "requestBody": body("RegisterInput"), "responses": map[string]any{"201": response("access_token、user"), "403": response("未开放注册"), "409": response("邮箱已被使用")}}},
		"/api/auth/login":    map[string]any{"post": map[string]any{"tags": user, "summary": "登录（与 /admin/api/auth/login 相同）", "security": public, "requestBody": body("LoginInput"), "responses": map[string]any{"200": response("access_token、user"), "401": response("邮箱或密码错误"), "403": response("账号已停用")}}},
		"/api/auth/logout":   map[string]any{"post": map[string]any{"tags": user, "summary": "退出", "responses": map[string]any{"200": response("已退出")}}},
		"/api/auth/me":       map[string]any{"get": map[string]any{"tags": user, "summary": "当前用户（含余额、角色、倍率）", "responses": map[string]any{"200": response("user")}}},
		"/api/user/overview": map[string]any{"get": map[string]any{"tags": user, "summary": "控制台概览", "responses": map[string]any{"200": response("余额、今日与 7 天消耗、模型分布、最近请求")}}},
		"/api/user/keys": map[string]any{
			"get":  map[string]any{"tags": user, "summary": "我的 API Key", "responses": map[string]any{"200": response("keys、max_keys")}},
			"post": map[string]any{"tags": user, "summary": "创建 API Key", "description": "完整密钥只在创建与重新生成时返回。受「每人 Key 上限」限制。", "requestBody": body("UserKeyInput"), "responses": map[string]any{"200": response("key"), "400": response("已达到 Key 数量上限或参数错误")}},
		},
		"/api/user/keys/{id}": map[string]any{
			"parameters": idParam,
			"patch":      map[string]any{"tags": user, "summary": "修改我的 Key（名称、启停、额度、清零已用）", "requestBody": body("UserKeyPatchInput"), "responses": map[string]any{"200": response("key"), "404": response("Key 不存在")}},
			"delete":     map[string]any{"tags": user, "summary": "删除我的 Key", "responses": map[string]any{"200": response("已删除")}},
		},
		"/api/user/keys/{id}/regenerate": map[string]any{"post": map[string]any{"tags": user, "summary": "重新生成我的 Key", "parameters": idParam, "responses": map[string]any{"200": response("key")}}},
		"/api/user/usage/records":        map[string]any{"get": map[string]any{"tags": user, "summary": "我的逐请求用量", "parameters": with(query("days", "最近天数", "integer"), query("key_id", "按 Key 筛选", "integer"), query("model", "按模型筛选", "string")), "responses": map[string]any{"200": response("days、total、records、totals")}}},
		"/api/user/usage/timeseries":     map[string]any{"get": map[string]any{"tags": user, "summary": "我的用量时间序列", "parameters": []any{query("days", "最近天数", "integer"), query("key_id", "按 Key 筛选", "integer")}, "responses": map[string]any{"200": response("bucket、points")}}},
		"/api/user/ledger":               map[string]any{"get": map[string]any{"tags": user, "summary": "我的余额流水", "parameters": with(query("kind", "类型", "string"), query("days", "最近天数", "integer")), "responses": map[string]any{"200": response("entries、total、sums、balance")}}},
		"/api/user/redeem": map[string]any{
			"get":  map[string]any{"tags": user, "summary": "我的兑换记录", "responses": map[string]any{"200": response("history、total")}},
			"post": map[string]any{"tags": user, "summary": "兑换充值码", "description": "大小写、空格与短横线不敏感；每个码只能兑换一次。连续失败 20 次后限制 15 分钟。", "requestBody": body("RedeemInput"), "responses": map[string]any{"200": response("amount、balance、entry"), "400": response("未填写兑换码"), "404": response("兑换码不存在"), "409": response("已被使用、已停用或已过期"), "429": response("尝试过多")}},
		},
		"/api/user/profile":  map[string]any{"patch": map[string]any{"tags": user, "summary": "修改昵称", "requestBody": body("ProfileInput"), "responses": map[string]any{"200": response("user")}}},
		"/api/user/password": map[string]any{"post": map[string]any{"tags": user, "summary": "修改密码", "description": "成功后其他会话失效，并返回新的 access_token。", "requestBody": body("ChangePasswordInput"), "responses": map[string]any{"200": response("access_token"), "400": response("原密码错误或新密码不符合要求")}}},
		"/api/user/models":   map[string]any{"get": map[string]any{"tags": user, "summary": "可用模型与对我生效的倍率", "responses": map[string]any{"200": response("models、user_multiplier")}}},
	}
	schemas := map[string]any{
		"UserCreateInput":       map[string]any{"type": "object", "required": []string{"email", "password"}, "properties": map[string]any{"email": map[string]any{"type": "string", "format": "email"}, "password": map[string]any{"type": "string", "minLength": 8, "writeOnly": true}, "nickname": str, "role": role, "balance": integer, "multiplier": multiplier, "note": str}},
		"UserPatchInput":        map[string]any{"type": "object", "properties": map[string]any{"nickname": str, "role": role, "enabled": boolean, "multiplier": multiplier, "note": str}},
		"PasswordInput":         map[string]any{"type": "object", "required": []string{"password"}, "properties": map[string]any{"password": map[string]any{"type": "string", "minLength": 8, "writeOnly": true}}},
		"BalanceAdjustInput":    map[string]any{"type": "object", "required": []string{"amount", "note"}, "properties": map[string]any{"amount": map[string]any{"type": "integer", "format": "int64", "description": "正数加款，负数扣款，不能为 0"}, "note": map[string]any{"type": "string", "minLength": 1}}},
		"NoteInput":             map[string]any{"type": "object", "properties": map[string]any{"note": str}},
		"RedeemCodeCreateInput": map[string]any{"type": "object", "required": []string{"amount", "count"}, "properties": map[string]any{"amount": map[string]any{"type": "integer", "minimum": 1, "description": "面值（Token）"}, "count": map[string]any{"type": "integer", "minimum": 1}, "expires_at": map[string]any{"type": "integer", "description": "Unix 秒，0 为永不过期"}, "batch": map[string]any{"type": "string", "description": "留空自动生成"}, "note": str}},
		"PlatformSettingsInput": map[string]any{"type": "object", "properties": map[string]any{"registration_open": boolean, "signup_bonus": integer, "default_user_multiplier": map[string]any{"type": "number", "minimum": 0}, "max_keys_per_user": map[string]any{"type": "integer"}, "site_name": str, "announcement": str, "video_tokens_per_second": integer, "user_unmetered_routes": map[string]any{"type": "boolean", "description": "是否允许用户 Key 调用不计 Token 的接口（Gemini 原生挂载、多模态透传），默认关闭"}}},
		"RegisterInput":         map[string]any{"type": "object", "required": []string{"email", "password"}, "properties": map[string]any{"email": map[string]any{"type": "string", "format": "email"}, "password": map[string]any{"type": "string", "minLength": 8, "writeOnly": true}, "nickname": str}},
		"UserKeyInput":          map[string]any{"type": "object", "properties": map[string]any{"name": str, "token_limit": map[string]any{"type": "integer", "description": "0 为不限（仍受余额约束）"}}},
		"UserKeyPatchInput":     map[string]any{"type": "object", "properties": map[string]any{"name": str, "enabled": boolean, "token_limit": integer, "reset_usage": boolean}},
		"RedeemInput":           map[string]any{"type": "object", "required": []string{"code"}, "properties": map[string]any{"code": map[string]any{"type": "string", "example": "W2A-XXXX-XXXX-XXXX-XXXX"}}},
		"ProfileInput":          map[string]any{"type": "object", "properties": map[string]any{"nickname": str}},
		"ChangePasswordInput":   map[string]any{"type": "object", "required": []string{"old_password", "new_password"}, "properties": map[string]any{"old_password": map[string]any{"type": "string", "writeOnly": true}, "new_password": map[string]any{"type": "string", "minLength": 8, "writeOnly": true}}},
	}
	for k, v := range paths {
		spec["paths"].(map[string]any)[k] = v
	}
	components := spec["components"].(map[string]any)
	for k, v := range schemas {
		components["schemas"].(map[string]any)[k] = v
	}
	spec["tags"] = append(spec["tags"].([]any),
		map[string]any{"name": "用户", "description": "平台用户、余额调整、余额流水与平台设置（需要 admin 角色）"},
		map[string]any{"name": "兑换码", "description": "批量生成、导出、停用与兑换记录"},
		map[string]any{"name": "用户控制台", "description": "/api/* 普通用户接口，与管理接口使用同一套 JWT"},
	)
	return spec
}
