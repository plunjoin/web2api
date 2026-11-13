import { cny } from '@/lib/format'

/** 额度条：used / limit 都是微元。 */
export function QuotaBar({ used, limit }: { used: number; limit: number }) {
  if (!limit) return <span className="text-muted-foreground">不限额 · 已用 {cny(used)}</span>
  const pct = Math.min(100, (used / limit) * 100)
  return (
    <div className="w-36">
      <div className="mb-1 flex justify-between text-[11px] text-muted-foreground tabular-nums">
        <span>{cny(used)}</span>
        <span>{cny(limit)}</span>
      </div>
      <div className="h-1 overflow-hidden rounded-full bg-white/[0.06]">
        <div className={`h-full rounded-full ${pct >= 100 ? 'bg-destructive' : pct > 80 ? 'bg-warning' : 'bg-primary'}`} style={{ width: `${Math.max(pct, 2)}%` }} />
      </div>
    </div>
  )
}
