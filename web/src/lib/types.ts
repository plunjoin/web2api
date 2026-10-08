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
  signup_bonus: number
  site_name: string
  version: string
}

export interface ApiKey {
  id: number
  key: string
  name: string
  enabled: boolean
  created_at: number
  token_limit: number
  tokens_used: number
  multiplier: number
  tokens_remaining: number | null
  quota_exhausted: boolean
  expires_at: number
  expired: boolean
  allowed_models: string[]
  rpm_limit: number
  user_id: number
  owner_email?: string
  requests_24h?: number
  charged_tokens_24h?: number
  charged_7d?: number
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
  charged_tokens: number
  error?: string
  latency_ms: number
  user_id: number
  user_email?: string
  refunded_tokens: number
}

export interface UsagePoint {
  ts: number
  requests: number
  success_requests: number
  total_tokens: number
  charged_tokens: number
  avg_latency_ms: number
}

export interface UsageTotals {
  requests: number
  success_requests: number
  total_tokens: number
  charged_tokens: number
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
  charged_7d?: number
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
  signup_bonus: number
  default_user_multiplier: number
  max_keys_per_user: number
  site_name: string
  announcement: string
  video_tokens_per_second: number
  user_unmetered_routes: boolean
}

export interface VersionInfo {
  version: string
  commit?: string
  build_time?: string
  modified?: boolean
  go_version: string
}

export interface AccountLive {
  id: string
  label: string
  enabled: boolean
  ready: boolean
  status: string
  detail?: string
  models: number
}

export interface Account {
  id: number
  engine: 'a' | 'b'
  label: string
  enabled: boolean
  status: string
  detail: string
  created_at: number
  updated_at: number
  live: AccountLive
}

export interface ModelInfo {
  id: string
  display_name?: string
  engine?: string
  available: boolean
  multiplier: number
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
