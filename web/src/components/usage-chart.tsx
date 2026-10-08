import { useMemo } from 'react'
import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from 'recharts'
import { ChartContainer, ChartTooltip, type ChartConfig } from '@/components/ui/chart'
import { axis, num } from '@/lib/format'
import type { UsagePoint } from '@/lib/types'
import { cn } from '@/lib/utils'
import { BarChart3 } from 'lucide-react'

export type UsageMetric = 'charged_tokens' | 'total_tokens' | 'requests'

const metricLabel: Record<UsageMetric, string> = {
  charged_tokens: '计费 Token',
  total_tokens: '总 Token',
  requests: '请求数',
}

function bucketLabel(ts: number, bucket: 'day' | 'hour') {
  const d = new Date(ts * 1000)
  return bucket === 'hour' ? `${String(d.getHours()).padStart(2, '0')}:00` : `${d.getMonth() + 1}/${d.getDate()}`
}

/**
 * 用量柱状图。稀疏数据也保持“诚实”：
 * - 每个时间桶都有一条浅色轨道（背景柱），单根柱子不会孤零零悬在空白里；
 * - 柱宽有上限，少量数据不会被拉成一整块；
 * - 全为 0 时不画坐标轴假装有数据，而是给出明确的空状态说明。
 */
export function UsageChart({ points, bucket = 'day', metric = 'charged_tokens', height = 220, className }: { points: UsagePoint[]; bucket?: 'day' | 'hour'; metric?: UsageMetric; height?: number; className?: string }) {
  const data = useMemo(
    () =>
      points.map((p) => ({
        label: bucketLabel(p.ts, bucket),
        ts: p.ts,
        value: p[metric],
        requests: p.requests,
        success: p.success_requests,
        charged: p.charged_tokens,
        total: p.total_tokens,
      })),
    [points, bucket, metric],
  )
  const nonZero = data.filter((d) => d.value > 0).length
  const config: ChartConfig = { value: { label: metricLabel[metric], color: 'var(--chart-1)' } }

  if (data.length === 0 || nonZero === 0) {
    return (
      <div className={cn('relative flex flex-col items-center justify-center text-center', className)} style={{ height }}>
        <div className="absolute inset-x-4 inset-y-6 flex items-end gap-1 opacity-60">
          {Array.from({ length: Math.max(data.length, 14) }).map((_, i) => (
            <div key={i} className="h-full flex-1 rounded-sm bg-white/[0.025]" />
          ))}
        </div>
        <div className="relative flex flex-col items-center">
          <BarChart3 className="mb-2 size-5 text-muted-foreground" />
          <p className="text-[13px] font-medium">该时间段还没有请求</p>
          <p className="mt-1 text-xs text-muted-foreground">产生调用后，这里会按{bucket === 'hour' ? '小时' : '天'}显示{metricLabel[metric]}。</p>
        </div>
      </div>
    )
  }

  const interval = data.length > 16 ? Math.ceil(data.length / 8) - 1 : 0
  return (
    <div className={className}>
      <ChartContainer config={config} className="aspect-auto w-full" style={{ height }}>
        <BarChart data={data} margin={{ top: 8, right: 8, left: 0, bottom: 0 }} barCategoryGap="18%">
          <CartesianGrid vertical={false} stroke="rgb(255 255 255 / 0.05)" />
          <XAxis dataKey="label" tickLine={false} axisLine={false} tickMargin={8} interval={interval} fontSize={11} />
          <YAxis tickLine={false} axisLine={false} width={44} tickFormatter={axis} fontSize={11} allowDecimals={false} />
          <ChartTooltip
            cursor={{ fill: 'rgb(255 255 255 / 0.04)' }}
            content={({ active, payload }) => {
              if (!active || !payload?.length) return null
              const d = payload[0].payload as (typeof data)[number]
              const date = new Date(d.ts * 1000)
              return (
                <div className="min-w-[180px] rounded-md border bg-popover px-3 py-2 text-xs shadow-xl">
                  <div className="mb-1.5 font-medium text-foreground">
                    {date.getMonth() + 1} 月 {date.getDate()} 日{bucket === 'hour' ? ` ${String(date.getHours()).padStart(2, '0')}:00` : ''}
                  </div>
                  <Row label="计费 Token" value={num(d.charged)} strong={metric === 'charged_tokens'} />
                  <Row label="总 Token" value={num(d.total)} strong={metric === 'total_tokens'} />
                  <Row label="请求" value={`${num(d.requests)}（成功 ${num(d.success)}）`} strong={metric === 'requests'} />
                </div>
              )
            }}
          />
          <Bar dataKey="value" fill="var(--color-value)" radius={[3, 3, 0, 0]} maxBarSize={28} background={{ fill: 'rgb(255 255 255 / 0.025)', radius: 3 }} />
        </BarChart>
      </ChartContainer>
      {nonZero <= 2 && data.length > 4 && <p className="px-1 pt-1 text-[11px] text-muted-foreground">数据较少：只有 {nonZero} 个时间段有调用。</p>}
    </div>
  )
}

function Row({ label, value, strong }: { label: string; value: string; strong?: boolean }) {
  return (
    <div className="flex items-center justify-between gap-4 py-0.5">
      <span className="flex items-center gap-1.5 text-muted-foreground">
        {strong && <span className="size-1.5 rounded-full bg-chart-1" />}
        {label}
      </span>
      <span className={cn('tabular-nums', strong ? 'font-medium text-foreground' : 'text-muted-foreground')}>{value}</span>
    </div>
  )
}

/** 横向占比条（模型 / Key 排行），比饼图更易读，数据少时也不会显得“坏掉”。 */
export function RankBars({ items, empty, format = num }: { items: { label: string; value: number; hint?: string }[]; empty?: string; format?: (v: number) => string }) {
  const max = Math.max(1, ...items.map((i) => i.value))
  const total = items.reduce((s, i) => s + i.value, 0)
  if (items.length === 0 || total === 0) {
    return <p className="px-4 py-8 text-center text-xs text-muted-foreground">{empty || '暂无数据'}</p>
  }
  return (
    <ul className="space-y-2.5 px-4 py-3">
      {items.map((item) => (
        <li key={item.label}>
          <div className="mb-1 flex items-baseline justify-between gap-3 text-[13px]">
            <span className="truncate font-mono text-xs text-foreground">{item.label}</span>
            <span className="shrink-0 tabular-nums text-muted-foreground">
              {format(item.value)}
              <span className="ml-1.5 text-[11px]">{((item.value / total) * 100).toFixed(item.value / total < 0.1 ? 1 : 0)}%</span>
            </span>
          </div>
          <div className="h-1.5 overflow-hidden rounded-full bg-white/[0.04]">
            <div className="h-full rounded-full bg-chart-1" style={{ width: `${Math.max(2, (item.value / max) * 100)}%` }} />
          </div>
        </li>
      ))}
    </ul>
  )
}
