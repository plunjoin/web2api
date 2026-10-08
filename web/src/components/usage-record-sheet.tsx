import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { KV } from '@/components/page'
import { Pill } from '@/components/status'
import { dateTime, latency, multiplier, num } from '@/lib/format'
import type { UsageRecord } from '@/lib/types'
import type { ReactNode } from 'react'

/** 单条请求详情：把扣费公式拆开展示，用户能看懂每个 Token 怎么来的。 */
export function UsageRecordSheet({ record, onOpenChange, admin, footer }: { record: UsageRecord | null; onOpenChange: (open: boolean) => void; admin?: boolean; footer?: ReactNode }) {
  return (
    <Sheet open={!!record} onOpenChange={onOpenChange}>
      <SheetContent className="w-full gap-0 sm:max-w-[440px]">
        {record && (
          <>
            <SheetHeader className="border-b">
              <SheetTitle className="flex items-center gap-2 text-[15px]">
                请求 #{record.id}
                {record.success ? <Pill tone="success">成功</Pill> : <Pill tone="danger">失败</Pill>}
                {record.refunded_tokens > 0 && <Pill tone="primary">已退款</Pill>}
              </SheetTitle>
              <SheetDescription className="text-xs">{dateTime(record.ts)}</SheetDescription>
            </SheetHeader>
            <div className="space-y-5 overflow-y-auto p-4">
              <KV
                items={[
                  ['模型', <span className="font-mono text-xs">{record.model || '—'}</span>],
                  ['接口', <span className="font-mono text-xs">{record.endpoint}</span>],
                  ['密钥', record.key_name ? `${record.key_name} · ${record.key_masked}` : record.key_masked || '—'],
                  ...(admin ? ([['用户', record.user_email || (record.user_id ? `#${record.user_id}` : '无归属')]] as [ReactNode, ReactNode][]) : []),
                  ['引擎', record.engine === 'a' ? 'Gemini 网页' : record.engine === 'b' ? 'AI Studio' : record.engine || '—'],
                  ['流式', record.stream ? '是' : '否'],
                  ['耗时', latency(record.latency_ms)],
                ]}
              />
              <div className="rounded-lg border">
                <div className="border-b px-3 py-2 text-xs font-medium">Token 与扣费</div>
                <div className="space-y-1.5 px-3 py-2.5 text-[13px]">
                  <Line label="输入 Token" value={num(record.prompt_tokens)} />
                  <Line label="输出 Token" value={num(record.completion_tokens)} />
                  <Line label="总 Token" value={num(record.total_tokens)} hint={record.estimated ? '本地估算（上游未返回用量）' : '上游返回'} />
                  <div className="my-2 border-t border-dashed" />
                  <Line label="模型倍率" value={multiplier(record.model_multiplier)} />
                  <Line label="Key 倍率" value={multiplier(record.key_multiplier)} />
                  <Line label="用户倍率" value={multiplier(record.user_multiplier || 1)} />
                  <div className="my-2 border-t border-dashed" />
                  <Line label="扣费 Token" value={<span className="font-semibold text-foreground">{num(record.charged_tokens)}</span>} />
                  <p className="pt-1 font-mono text-[11px] text-muted-foreground">
                    ⌈{num(record.total_tokens)} × {Number(record.multiplier.toFixed(6))}⌉ = {num(record.charged_tokens)}
                  </p>
                  {record.refunded_tokens > 0 && <Line label="已退款" value={`+${num(record.refunded_tokens)}`} />}
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
