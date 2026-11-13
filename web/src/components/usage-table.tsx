import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { Pill, StatusDot } from '@/components/status'
import { clock, cny, compact, dateTime, usd } from '@/lib/format'
import { useCNYPerUSD } from '@/lib/currency'
import { engineLabel } from '@/lib/types'
import type { UsageRecord } from '@/lib/types'

/** 供应商类型标签：订阅号池 / 网页账号 / 第三方 Key。 */
function accountKind(engine: string) {
  const name = engine.startsWith('custom|') ? 'custom' : engine
  if (!name) return { label: '未归属', tone: 'muted' as const }
  if (name === 'custom') return { label: '第三方 Key', tone: 'warning' as const }
  if (name === 'geminiweb' || name === 'aistudio') return { label: '网页账号', tone: 'muted' as const }
  return { label: '订阅账号', tone: 'primary' as const }
}

/** 缓存命中率：cached / max(prompt, cached)，兼容「prompt 已含缓存」与「已扣除缓存」两种上游口径。 */
function cacheRate(r: UsageRecord) {
  if (!r.prompt_tokens) return '—'
  if (!r.cached_tokens) return '0%'
  const pct = Math.min(100, (r.cached_tokens / Math.max(r.prompt_tokens, r.cached_tokens)) * 100)
  return `${pct >= 99.95 ? 100 : Number(pct.toFixed(1))}%`
}

/** 毫秒计时按「秒」展示（与请求日志的阅读习惯一致，毫秒级才保留 ms）。 */
function elapsed(ms: number) {
  if (ms <= 0) return '—'
  return ms >= 1000 ? `${(ms / 1000).toFixed(1)} 秒` : `${ms} ms`
}

/** 生成速度：输出 Token / 首字之后的时间。 */
function speed(r: UsageRecord) {
  if (!r.success || r.completion_tokens <= 0 || r.latency_ms <= 0) return '—'
  const genMs = Math.max(1, r.latency_ms - (r.ttft_ms || 0))
  return `${Math.round(r.completion_tokens / (genMs / 1000))} token/秒`
}

export function UsageTable({ records, onSelect, showUser }: { records: UsageRecord[]; onSelect: (r: UsageRecord) => void; showUser?: boolean }) {
  const rate = useCNYPerUSD()
  return (
    <div className="overflow-x-auto">
      <Table>
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            <TableHead>时间</TableHead>
            <TableHead className="hidden md:table-cell">AGENT</TableHead>
            <TableHead>请求模型</TableHead>
            <TableHead>供应商 · 账号</TableHead>
            <TableHead className="hidden xl:table-cell">实际模型</TableHead>
            <TableHead className="text-right">输入</TableHead>
            <TableHead className="text-right">输出</TableHead>
            <TableHead className="hidden text-right md:table-cell">缓存命中率</TableHead>
            <TableHead className="text-right">费用</TableHead>
            <TableHead className="hidden text-right lg:table-cell">首字耗时</TableHead>
            <TableHead className="hidden text-right md:table-cell">耗时</TableHead>
            <TableHead className="hidden text-right xl:table-cell">速度</TableHead>
            <TableHead>状态</TableHead>
            {showUser && <TableHead className="hidden 2xl:table-cell">用户</TableHead>}
          </TableRow>
        </TableHeader>
        <TableBody>
          {records.map((r) => {
            const kind = accountKind(r.engine)
            const provider = r.provider || engineLabel(r.engine)
            return (
              <TableRow key={r.id} className="cursor-pointer" onClick={() => onSelect(r)}>
                <TableCell className="whitespace-nowrap text-muted-foreground tabular-nums">
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <span>{clock(r.ts)}</span>
                    </TooltipTrigger>
                    <TooltipContent>{dateTime(r.ts)}</TooltipContent>
                  </Tooltip>
                </TableCell>
                <TableCell className="hidden max-w-[120px] truncate text-[13px] md:table-cell" title={r.client}>
                  {r.client || '—'}
                </TableCell>
                <TableCell className="max-w-[160px] truncate font-mono text-xs" title={r.model}>
                  {r.model || '—'}
                </TableCell>
                <TableCell>
                  <div className="flex max-w-[260px] items-center gap-1.5 text-[13px]">
                    <span className="shrink-0 text-muted-foreground">{provider}</span>
                    {r.account_label && (
                      <>
                        <span className="shrink-0 text-muted-foreground/50">·</span>
                        <span className="truncate" title={r.account_label}>
                          {r.account_label}
                        </span>
                      </>
                    )}
                  </div>
                  <div className="mt-1 flex flex-wrap gap-1">
                    <Pill tone={kind.tone}>{kind.label}</Pill>
                    {r.endpoint === '/v1/responses' && <Pill tone="muted">Responses → Chat</Pill>}
                    {r.stream && <Pill tone="muted">流式</Pill>}
                  </div>
                </TableCell>
                <TableCell className="hidden max-w-[160px] truncate font-mono text-xs text-muted-foreground xl:table-cell" title={r.actual_model}>
                  {r.actual_model || r.model || '—'}
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {compact(r.prompt_tokens)}
                  {r.estimated && <span className="ml-1 text-[11px] text-muted-foreground" title="本地估算">≈</span>}
                </TableCell>
                <TableCell className="text-right tabular-nums">{compact(r.completion_tokens)}</TableCell>
                <TableCell className="hidden text-right text-muted-foreground tabular-nums md:table-cell" title={r.prompt_tokens && !r.cached_tokens ? '上游没有报告缓存命中' : undefined}>
                  {cacheRate(r)}
                </TableCell>
                <TableCell className="whitespace-nowrap text-right tabular-nums">
                  <span className="font-medium">{cny(r.cost_micros)}</span>
                  <div className="text-[11px] text-muted-foreground">≈ {usd(r.cost_micros, rate)}</div>
                  {r.refunded_micros > 0 && <div className="text-[11px] text-muted-foreground">已退 {cny(r.refunded_micros)}</div>}
                </TableCell>
                <TableCell className="hidden text-right text-muted-foreground tabular-nums lg:table-cell">{elapsed(r.ttft_ms)}</TableCell>
                <TableCell className="hidden text-right text-muted-foreground tabular-nums md:table-cell">{elapsed(r.latency_ms)}</TableCell>
                <TableCell className="hidden text-right text-muted-foreground tabular-nums xl:table-cell">{speed(r)}</TableCell>
                <TableCell>
                  {r.status_code > 0 ? (
                    <StatusDot tone={r.success ? 'success' : 'danger'}>{r.status_code}</StatusDot>
                  ) : r.success ? (
                    <StatusDot tone="success">成功</StatusDot>
                  ) : (
                    <StatusDot tone="danger">失败</StatusDot>
                  )}
                </TableCell>
                {showUser && (
                  <TableCell className="hidden max-w-[180px] truncate text-muted-foreground 2xl:table-cell">
                    {r.user_email || r.key_name || r.key_masked || (r.user_id ? `#${r.user_id}` : '无归属')}
                  </TableCell>
                )}
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
    </div>
  )
}
