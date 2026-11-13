import { useState } from 'react'
import { ChevronDown, ChevronRight, RefreshCw } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { RowMenu, type RowAction } from '@/components/row-menu'
import { AccountStatus, CooldownLines, TierPill, accountHealth } from '@/components/account-status'
import { StatusDot } from '@/components/status'
import { canonicalEngine, engineLabel, type Account, type QuotaWindow } from '@/lib/types'
import { aggregateQuota, freshQuota, knownWindow, quotaLabel } from '@/lib/account-quota'
import { relative } from '@/lib/format'

const ORDER = ['geminiweb', 'aistudio', 'codex', 'antigravity', 'kiro', 'workbuddy', 'custom']
const UNKNOWN = 'custom|unknown'
const format = (v: number) => new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 2 }).format(v)

function groupKey(a: Account): string {
  const engine = canonicalEngine(a.engine)
  if (engine === 'custom') return a.provider || UNKNOWN
  return engine === 'workbuddy' ? a.live?.service || engine : engine
}

function groupLabel(key: string, engine: string): string {
  return engine === 'custom' ? key === UNKNOWN ? '第三方 Key（未识别地址）' : key : engineLabel(key)
}

function QuotaFigures({ windows }: { windows: QuotaWindow[] }) {
  return (
    <dl className="grid gap-x-5 gap-y-3 sm:grid-cols-2">
      {windows.map((w) => {
        const remaining = Math.min(w.limit, w.remaining)
        const percent = Math.round(remaining / w.limit * 1000) / 10
        const label = quotaLabel(w.name)
        return (
          <div key={`${w.id}-${w.unit}-${w.aside}`} className="min-w-0">
            <dt className="text-xs text-muted-foreground">{label}</dt>
            <dd className="mt-1 text-xs tabular-nums">
              {format(remaining)} / {format(w.limit)} {w.unit === 'credits' ? '积分' : w.unit} · 剩余 {percent}%
            </dd>
            <div role="meter" aria-label={`${label}剩余`} aria-valuemin={0} aria-valuemax={w.limit} aria-valuenow={remaining} aria-valuetext={`${percent}%`} title={`${label}：${format(remaining)} / ${format(w.limit)}`} className="mt-1.5 h-1 overflow-hidden rounded-full bg-muted">
              <div className="h-full rounded-full bg-primary" style={{ width: `${percent}%` }} />
            </div>
          </div>
        )
      })}
    </dl>
  )
}

type CheckInControls = {
  checkingIn: number | null
  onCheckIn: (a: Account) => void
  onToggleAutoCheckIn: (a: Account, enabled: boolean) => void
}

function AccountCard({ account: a, checking, onCheck, actions, checkin }: {
  account: Account; checking: boolean; onCheck: (a: Account) => void; actions: RowAction[]
  checkin?: CheckInControls
}) {
  const [expanded, setExpanded] = useState(false)
  const snapshot = a.live?.quota_snapshot
  const isFresh = freshQuota(snapshot)
  const windows = snapshot?.windows?.filter(knownWindow) || []
  const detail = a.live?.detail || a.detail
  return (
    <article className="min-w-0 border-t px-4 py-4">
      <div className="flex items-start gap-2">
        <button type="button" onClick={() => setExpanded(!expanded)} aria-expanded={expanded} className="flex min-h-8 min-w-0 flex-1 items-center gap-2 rounded text-left focus-visible:outline-2 focus-visible:outline-ring">
          {expanded ? <ChevronDown aria-hidden="true" className="size-3.5 shrink-0" /> : <ChevronRight aria-hidden="true" className="size-3.5 shrink-0" />}
          <span className="min-w-0 break-words text-sm font-medium [overflow-wrap:anywhere]">{a.label}</span>
        </button>
        <TierPill tier={a.live?.tier} />
        <RowMenu actions={actions} />
      </div>
      <div className="mt-1 flex flex-wrap items-center gap-2 text-xs">
        {checking ? <StatusDot tone="warning">检查中…</StatusDot> : <AccountStatus account={a} />}
        <span className="text-muted-foreground">{a.live?.models ?? 0} 个模型</span>
        {snapshot && <span className="text-muted-foreground">额度{isFresh ? '已更新' : '待刷新'}</span>}
      </div>
      {a.live?.auto_checkin !== undefined && checkin && (
        <div className="mt-2 flex flex-wrap items-center gap-3">
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            <Switch checked={a.live?.auto_checkin ?? true} onCheckedChange={(v) => checkin.onToggleAutoCheckIn(a, v)} aria-label="自动签到" />
            <span>自动签到</span>
          </div>
          <Button size="sm" variant="outline" onClick={() => checkin.onCheckIn(a)} disabled={checkin.checkingIn === a.id}>
            <RefreshCw aria-hidden="true" className={checkin.checkingIn === a.id ? 'animate-spin' : ''} /> 立即签到
          </Button>
          {a.live?.check_in && <span className="text-xs text-muted-foreground">{a.live.check_in}</span>}
        </div>
      )}
      <div className="mt-3">
        {windows.length > 0 ? (
          <div className={isFresh ? '' : 'opacity-60'}><QuotaFigures windows={windows} /></div>
        ) : <p className="break-words text-xs text-muted-foreground [overflow-wrap:anywhere]">{snapshot?.error ? `额度未知：${snapshot.error}` : a.live?.quota || '上游未提供可计算额度'}</p>}
        {!isFresh && windows.length > 0 && <p className="mt-2 text-xs text-muted-foreground">历史快照，不计入可用额度汇总。</p>}
      </div>
      {expanded && (
        <div className="mt-3 space-y-2 text-xs text-muted-foreground">
          {detail && <p className="break-words [overflow-wrap:anywhere]">{detail}</p>}
          <CooldownLines cooldowns={a.live?.cooldowns} />
          {snapshot?.error && <p role="status">额度获取失败：{snapshot.error}</p>}
          {windows.map((w) => (w.resets_at || w.expires_at) && (
            <div key={`${w.id}-${w.unit}`} className="space-y-1">
              {w.resets_at && <p>{quotaLabel(w.name)}重置于 {new Date(w.resets_at).toLocaleString('zh-CN')}</p>}
              {w.expires_at && <p>{format(w.expiring || 0)} {w.unit} 将于 {new Date(w.expires_at).toLocaleString('zh-CN')} 到期</p>}
            </div>
          ))}
          <div className="flex flex-wrap items-center justify-between gap-2">
            <span>#{a.id} · {snapshot?.read_at ? `额度采样 ${relative(snapshot.read_at)}` : `账号更新 ${relative(a.updated_at)}`}</span>
            <Button size="sm" variant="outline" onClick={() => onCheck(a)} disabled={checking}><RefreshCw aria-hidden="true" className={checking ? 'animate-spin' : ''} />立即检查</Button>
          </div>
        </div>
      )}
    </article>
  )
}

export function ProviderGroups({ accounts, checking, collapsed, onToggle, onOpen, onCheck, actions, checkin }: {
  accounts: Account[]; checking: number | null; collapsed: Set<string>
  onToggle: (key: string) => void
  onOpen: (target: { engine: string; provider?: string }) => void
  onCheck: (a: Account) => void; actions: (a: Account) => RowAction[]
  checkin?: CheckInControls
}) {
  const groups = new Map<string, Account[]>()
  for (const a of accounts) {
    const key = groupKey(a)
    groups.set(key, [...(groups.get(key) || []), a])
  }
  const keys = [...groups.keys()].sort((x, y) => {
    const rank = (key: string) => { const i = ORDER.indexOf(canonicalEngine(groups.get(key)![0].engine)); return i < 0 ? 99 : i }
    return rank(x) - rank(y) || x.localeCompare(y)
  })
  return (
    <div className="grid items-start gap-4 xl:grid-cols-2">
      {[0, 1].map((column) => (
        <div key={column} className="min-w-0 space-y-4">
      {keys.filter((_, i) => i % 2 === column).map((key) => {
        const list = groups.get(key)!
        const engine = canonicalEngine(list[0].engine)
        const up = list.filter((a) => accountHealth(a) === 'up').length
        const broken = list.filter((a) => accountHealth(a) === 'down').length
        const cooling = list.filter((a) => a.live?.cooldowns?.length).length
        const summary = engine === 'workbuddy' ? aggregateQuota(list) : null
        const isCollapsed = collapsed.has(key)
        return (
          <section key={key} aria-label={groupLabel(key, engine)} className="min-w-0 overflow-hidden rounded-xl border bg-card">
            <header className="flex items-start gap-2 px-4 pt-4 pb-3">
              <button type="button" className="flex min-h-8 min-w-0 flex-1 items-start gap-2 rounded text-left focus-visible:outline-2 focus-visible:outline-ring" onClick={() => onToggle(key)} aria-expanded={!isCollapsed}>
                {isCollapsed ? <ChevronRight aria-hidden="true" className="mt-1 size-4 shrink-0 text-muted-foreground" /> : <ChevronDown aria-hidden="true" className="mt-1 size-4 shrink-0 text-muted-foreground" />}
                <span className="min-w-0">
                  <span className="block break-words text-sm font-semibold [overflow-wrap:anywhere]">{groupLabel(key, engine)}</span>
                  <span className="mt-1 block text-xs font-normal text-muted-foreground">{list.length} 个账号 · 可用 {up}{cooling ? ` · 冷却 ${cooling}` : ''}{broken ? ` · 异常 ${broken}` : ''}</span>
                </span>
              </button>
              <Button size="sm" variant="outline" disabled={key === UNKNOWN} onClick={() => onOpen({ engine, provider: engine === 'custom' ? key : undefined })}>{engine === 'workbuddy' ? '引擎模型' : '模型'}</Button>
            </header>
            {summary && (
              <div className="mx-4 mb-4 rounded-lg bg-muted/40 p-3">
                <p className="mb-3 text-xs text-muted-foreground">可用账号额度 · 已知 {summary.covered}/{summary.eligible} 个账号</p>
                {summary.windows.length ? <QuotaFigures windows={summary.windows} /> : <p className="text-xs text-muted-foreground">暂无新鲜额度数据，未知额度不按零计算。</p>}
                {summary.covered < summary.eligible && <p className="mt-2 text-xs text-muted-foreground">仅汇总已知部分，不代表完整余额。</p>}
              </div>
            )}
            {!isCollapsed && list.map((a) => <AccountCard key={a.id} account={a} checking={checking === a.id} onCheck={onCheck} actions={actions(a)} checkin={checkin} />)}
          </section>
        )
      })}
        </div>
      ))}
    </div>
  )
}
