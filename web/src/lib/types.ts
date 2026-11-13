export type Role = 'admin' | 'user'

export interface Me {
  id: number
  email: string
  nickname: string
  role: Role
  kind: 'root' | 'user'
  balance?: number
  multiplier?: number
  created_at?: number
}

export interface PublicConfig {
  initialized: boolean
  registration_open: boolean
  signup_bonus_micros: number
  cny_per_usd: number
  site_name: string
  version: string
}

/** 模型单价（人民币元 / 百万 Token）；cache_read = -1 表示按输入价。 */
export interface ModelPrice {
  model: string
  input: number
  output: number
  cache_read: number
  updated_at: number
}

export interface ApiKey {
  id: number
  key: string
  name: string
  enabled: boolean
  created_at: number
  cost_limit: number
  cost_used: number
  multiplier: number
  cost_remaining: number | null
  quota_exhausted: boolean
  expires_at: number
  expired: boolean
  allowed_models: string[]
  rpm_limit: number
  user_id: number
  owner_email?: string
  requests_24h?: number
  cost_micros_24h?: number
  cost_7d?: number
  requests_7d?: number
}

export interface UsageRecord {
  id: number
  ts: number
  key_id: number
  key_name: string
  key_masked: string
  engine: string
  model: string
  endpoint: string
  stream: boolean
  success: boolean
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  estimated: boolean
  model_multiplier: number
  key_multiplier: number
  user_multiplier: number
  multiplier: number
  cost_micros: number
  error?: string
  latency_ms: number
  /** 调用方客户端（由 User-Agent 识别，如 Claude Code） */
  client?: string
  /** 上游实际承接的模型；与 model（请求模型）不同时单独展示 */
  actual_model?: string
  account_id: number
  account_label?: string
  provider?: string
  /** 返回给客户端的 HTTP 状态码；0 表示旧记录未采集 */
  status_code: number
  /** 流式首字耗时；0 表示非流式或未采集 */
  ttft_ms: number
  /** 上游命中的缓存 Prompt Token（含在 prompt_tokens 内） */
  cached_tokens: number
  user_id: number
  user_email?: string
  refunded_micros: number
}

export interface UsagePoint {
  ts: number
  requests: number
  success_requests: number
  total_tokens: number
  cost_micros: number
  avg_latency_ms: number
}

export interface UsageTotals {
  requests: number
  success_requests: number
  total_tokens: number
  cost_micros: number
}

export type LedgerKind = 'adjust' | 'redeem' | 'usage' | 'refund' | 'signup_bonus'

export interface LedgerEntry {
  id: number
  ts: number
  user_id: number
  user_email?: string
  kind: LedgerKind
  amount: number
  balance_before: number
  balance_after: number
  ref_id: number
  note: string
  operator: string
}

export interface User {
  id: number
  email: string
  nickname: string
  role: Role
  enabled: boolean
  balance: number
  multiplier: number
  note: string
  created_at: number
  updated_at: number
  last_login_at: number
  key_count?: number
  cost_7d?: number
  requests_7d?: number
  total_recharge?: number
}

export interface RedeemCode {
  id: number
  code: string
  amount: number
  batch: string
  note: string
  enabled: boolean
  expires_at: number
  created_at: number
  created_by: string
  redeemed_by: number
  redeemed_email: string
  redeemed_at: number
  status: 'unused' | 'redeemed' | 'disabled' | 'expired'
}

export interface CodeStats {
  total: number
  unused: number
  redeemed: number
  unused_amount: number
  redeemed_amount: number
}

export interface PlatformSettings {
  registration_open: boolean
  signup_bonus_micros: number
  default_user_multiplier: number
  max_keys_per_user: number
  site_name: string
  announcement: string
  video_price_micros_per_second: number
  cny_per_usd: number
  user_unmetered_routes: boolean
}

export interface VersionInfo {
  version: string
  commit?: string
  build_time?: string
  modified?: boolean
  go_version: string
}

export interface AccountCooldown {
  scope: string
  until: number
  reason?: string
}

export interface QuotaWindow {
  id: string
  name: string
  unit: string
  limit: number
  remaining: number
  resets_at?: string
  expires_at?: string
  expiring?: number
  aside?: boolean
}

export interface QuotaSnapshot {
  read_at: number
  error?: string
  stale?: boolean
  windows?: QuotaWindow[]
}

export interface AccountLive {
  id: string
  label: string
  enabled: boolean
  ready: boolean
  status: string
  detail?: string
  models: number
  tier?: string
  quota?: string
  service?: string
  quota_snapshot?: QuotaSnapshot
  cooldowns?: AccountCooldown[]
  auto_checkin?: boolean
  /** 最近一次签到结果文案（teleagent / workbuddy 国服） */
  check_in?: string
}

// 引擎标识：a=Gemini Web、b=AI Studio；codex=GPT、antigravity、订阅引擎 + custom 第三方 Key。
export type EngineName = 'geminiweb' | 'aistudio' | 'codex' | 'antigravity' | 'kiro' | 'workbuddy' | 'teleagent' | 'custom'

export function canonicalEngine(engine: string): string {
  return engine === 'a' ? 'geminiweb' : engine === 'b' ? 'aistudio' : engine
}

export interface EngineMeta {
  name: string
  describe: string
  enabled: boolean
  account_pool: boolean
  oauth?: string
  credential?: string
}

// 引擎显示名映射（后端未启用该引擎时仍可展示存量账号）。
export const ENGINE_LABELS: Record<string, string> = {
  a: 'Gemini Web',
  b: 'AI Studio',
  geminiweb: 'Gemini Web',
  aistudio: 'AI Studio',
  'workbuddy-ai': 'WorkBuddy AI',
  codex: 'GPT',
  antigravity: 'Antigravity',
  kiro: 'Kiro',
  workbuddy: 'WorkBuddy',
  teleagent: 'TeleAgent',
  custom: '第三方 Key',
}

export function engineLabel(engine: string): string {
  // 第三方 Key 的模型快照按上游地址分开存，键是 "custom|<base_url>"：展示地址本身。
  if (engine.startsWith('custom|')) return engine.slice('custom|'.length) || ENGINE_LABELS.custom
  return ENGINE_LABELS[engine] || engine
}

export interface Account {
  id: number
  engine: string
  /** 第三方 Key 的供应商地址（归一化 base_url）；同一地址的多个 Key 属于同一个供应商。 */
  provider?: string
  label: string
  enabled: boolean
  status: string
  detail: string
  created_at: number
  updated_at: number
  live: AccountLive
}

export interface RoutingAccount {
  id: number
  label: string
  engine: string
  provider?: string
  enabled: boolean
  ready: boolean
  status: string
  models: number
}

export interface RoutingSupplier {
  key: string
  engine: string
  provider?: string
  accounts: RoutingAccount[]
}

export interface RoutingView {
  /** 是否保存过自定义顺序；false 时展示的是默认顺序 */
  configured: boolean
  /** 拍平后的账号 ID 顺序（提交时原样回传） */
  order: number[]
  suppliers: RoutingSupplier[]
  /** 会话粘性：off 关闭；session 同一会话优先留在上次接单的账号 */
  affinity: 'off' | 'session'
}

export interface ModelInfo {
  id: string
  display_name?: string
  engine?: string
  available: boolean
  /** 管理台上下架：false 时下游 /v1/models 不可见 */
  enabled?: boolean
  kind?: string
  /** 上游账号权益内的免费模型（不计套餐积分） */
  free?: boolean
  multiplier: number
  last_check_at?: number
  last_check_ok?: boolean
  last_check_detail?: string
}

export interface UpgradeState {
  enabled: boolean
  reason?: string
  image: string
  current_id?: string
  current_version?: string
  phase?: string
  message?: string
  target_id?: string
  target_version?: string
  error?: string
  [k: string]: unknown
}
