import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { KV } from '@/components/page'
import { Pill } from '@/components/status'
import { cny, dateTime, multiplier, num, usd } from '@/lib/format'
import { useCNYPerUSD } from '@/lib/currency'
import { engineLabel } from '@/lib/types'
import type { UsageRecord } from '@/lib/types'
import type { ReactNode } from 'react'

function elapsed(ms: number) {
  if (ms <= 0) return '—'
  return ms >= 1000 ? `${(ms / 1000).toFixed(1)} 秒` : `${ms} ms`
}

function cacheRate(record: UsageRecord) {
  const cached = record.cached_tokens || 0
  if (!record.prompt_tokens) return '—'
  if (!cached) return '0%'
  const pct = Math.min(100, (cached / Math.max(record.prompt_tokens, cached)) * 100)
  return `${pct >= 99.95 ? 100 : Number(pct.toFixed(1))}%`
}

function genSpeed(record: UsageRecord) {
  if (!record.success || record.completion_tokens <= 0 || record.latency_ms <= 0) return '—'
  const genMs = Math.max(1, record.latency_ms - (record.ttft_ms || 0))
  return `${Math.round(record.completion_tokens / (genMs / 1000))} token/秒`
}

/** 单条请求详情：把扣费公式拆开展示，用户能看懂每个 Token 怎么来的。 */
export function UsageRecordSheet({ record, onOpenChange, admin, footer }: { record: UsageRecord | null; onOpenChange: (open: boolean) => void; admin?: boolean; footer?: ReactNode }) {
  const rate = useCNYPerUSD()
  return (
    <Sheet open={!!record} onOpenChange={onOpenChange}>
      <SheetContent className="w-full gap-0 sm:max-w-[440px]">
        {record && (
          <>
            <SheetHeader className="border-b">
              <SheetTitle className="flex items-center gap-2 text-[15px]">
                请求 #{record.id}
                {record.success ? <Pill tone="success">成功</Pill> : <Pill tone="danger">失败</Pill>}
                {record.refunded_micros > 0 && <Pill tone="primary">已退款</Pill>}
              </SheetTitle>
              <SheetDescription className="text-xs">{dateTime(record.ts)}</SheetDescription>
            </SheetHeader>
            <div className="space-y-5 overflow-y-auto p-4">
              <KV
                items={[
                  ['请求模型', <span className="font-mono text-xs">{record.model || '—'}</span>],
                  ...(record.actual_model && record.actual_model !== record.model
                    ? ([['实际模型', <span className="font-mono text-xs">{record.actual_model}</span>]] as [ReactNode, ReactNode][])
                    : []),
                  ['客户端', record.client || '—'],
                  ['接口', <span className="font-mono text-xs">{record.endpoint}</span>],
                  ['密钥', record.key_name ? `${record.key_name} · ${record.key_masked}` : record.key_masked || '—'],
                  ...(admin ? ([['用户', record.user_email || (record.user_id ? `#${record.user_id}` : '无归属')]] as [ReactNode, ReactNode][]) : []),
                  ['引擎', record.engine ? engineLabel(record.engine) : '—'],
                  ...(record.account_label || record.provider
                    ? ([['供应商 · 账号', [record.provider, record.account_label].filter(Boolean).join(' · ')]] as [ReactNode, ReactNode][])
                    : []),
                  ['状态码', record.status_code || '—'],
                  ['流式', record.stream ? '是' : '否'],
                  ['首字耗时', elapsed(record.ttft_ms)],
                  ['耗时', elapsed(record.latency_ms)],
                  ['生成速度', genSpeed(record)],
                ]}
              />
              <div className="rounded-lg border">
                <div className="border-b px-3 py-2 text-xs font-medium">Token 与费用</div>
                <div className="space-y-1.5 px-3 py-2.5 text-[13px]">
                  <Line label="输入 Token" value={num(record.prompt_tokens)} />
                  {(record.cached_tokens || 0) > 0 && <Line label="其中缓存命中" value={`${num(record.cached_tokens)}（${cacheRate(record)}）`} />}
                  <Line label="输出 Token" value={num(record.completion_tokens)} />
                  <Line label="总 Token" value={num(record.total_tokens)} hint={record.estimated ? '本地估算（上游未返回用量）' : '上游返回'} />
                  <div className="my-2 border-t border-dashed" />
                  <Line label="模型倍率" value={multiplier(record.model_multiplier)} />
                  <Line label="Key 倍率" value={multiplier(record.key_multiplier)} />
                  <Line label="用户倍率" value={multiplier(record.user_multiplier || 1)} />
                  <div className="my-2 border-t border-dashed" />
                  <Line
                    label="应付费用"
                    value={
                      <span className="font-semibold text-foreground">
                        {cny(record.cost_micros)} <span className="text-[11px] font-normal text-muted-foreground">≈ {usd(record.cost_micros, rate)}</span>
                      </span>
                    }
                  />
                  <p className="pt-1 font-mono text-[11px] text-muted-foreground">
                    ⌈（未缓存输入 × 输入单价 + 缓存输入 × 缓存单价 + 输出 × 输出单价）× {Number(record.multiplier.toFixed(6))}⌉ = {cny(record.cost_micros)}
                  </p>
                  {record.refunded_micros > 0 && <Line label="已退款" value={`+${cny(record.refunded_micros)}`} />}
                </div>
              </div>
              {record.error && (
                <div>
                  <div className="mb-1.5 text-xs font-medium">错误信息</div>
                  <pre className="max-h-48 overflow-auto rounded-md border border-destructive/20 bg-destructive/5 p-2.5 font-mono text-[11px] leading-relaxed whitespace-pre-wrap text-[#ff9a9d]">{record.error}</pre>
                </div>
              )}
              {footer}
            </div>
          </>
        )}
      </SheetContent>
    </Sheet>
  )
}

function Line({ label, value, hint }: { label: string; value: ReactNode; hint?: string }) {
  return (
    <div className="flex items-baseline justify-between gap-3">
      <span className="text-muted-foreground">
        {label}
        {hint && <span className="ml-1.5 text-[11px] text-muted-foreground/70">{hint}</span>}
      </span>
      <span className="tabular-nums">{value}</span>
    </div>
  )
}
