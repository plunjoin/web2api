import { StatusDot } from '@/components/status'
import type { Account } from '@/lib/types'

/** 号池账号状态：busy（正在处理请求）、cooldown（限流冷却）不是故障。 */
export const accountStatusMeta: Record<string, { label: string; tone: 'success' | 'warning' | 'danger' | 'muted' }> = {
  ok: { label: '就绪', tone: 'success' },
  ready: { label: '就绪', tone: 'success' },
  busy: { label: '处理中', tone: 'success' },
  cooldown: { label: '冷却中', tone: 'warning' },
  initializing: { label: '初始化中', tone: 'warning' },
  unloaded: { label: '未加载', tone: 'warning' },
  unknown: { label: '未知', tone: 'muted' },
  disabled: { label: '已停用', tone: 'muted' },
  error: { label: '异常', tone: 'danger' },
}

export function accountHealth(a: Account): 'up' | 'degraded' | 'down' | 'off' {
  if (!a.enabled) return 'off'
  const s = a.live?.status || a.status
  if (a.live?.ready || s === 'ok' || s === 'busy') return 'up'
  if (s === 'cooldown' || s === 'initializing' || s === 'unloaded') return 'degraded'
  return 'down'
}

export function AccountStatus({ account }: { account: Account }) {
  if (!account.enabled) return <StatusDot tone="muted">已停用</StatusDot>
  const s = account.live?.status || account.status
  const meta = accountStatusMeta[s] || { label: s, tone: account.live?.ready ? 'success' : 'danger' }
  return <StatusDot tone={meta.tone}>{meta.label}</StatusDot>
}
