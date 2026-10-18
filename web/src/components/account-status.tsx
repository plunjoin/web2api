import { StatusDot, Pill } from '@/components/status'
import { relative } from '@/lib/format'
import type { Account, AccountCooldown } from '@/lib/types'

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

const tierNames: Record<string, string> = { Free: '免费', Plus: 'Plus', Pro: 'Pro', Ultra: 'Ultra' }

export function tierLabel(tier?: string) {
  if (!tier) return ''
  return tierNames[tier] || tier
}

export function TierPill({ tier }: { tier?: string }) {
  if (!tier) return <span className="text-xs text-muted-foreground">—</span>
  const tone = tier === 'Ultra' || tier === 'Pro' ? 'primary' : tier === 'Plus' ? 'success' : 'muted'
  return <Pill tone={tone}>{tierLabel(tier)}</Pill>
}

function cooldownScope(scope: string) {
  if (!scope || scope === '*') return '全部模型'
  if (scope.startsWith('build:')) return `Build · ${scope.slice(6)}`
  return scope
}

export function CooldownLines({ cooldowns }: { cooldowns?: AccountCooldown[] }) {
  if (!cooldowns?.length) return null
  return (
    <div className="mt-1 space-y-0.5">
      {cooldowns.map((item) => (
        <div key={`${item.scope}-${item.until}`} className="truncate text-[11px] text-muted-foreground" title={item.reason || undefined}>
          {cooldownScope(item.scope)} · {relative(item.until)}恢复
        </div>
      ))}
    </div>
  )
}
