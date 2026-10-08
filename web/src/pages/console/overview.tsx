import { useState } from 'react'
import { Link } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { Activity, CalendarDays, KeyRound, Megaphone, Plus, Ticket, Wallet } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { PageHeader, Section } from '@/components/page'
import { StatStrip } from '@/components/stat'
import { EmptyState, ErrorState, TableSkeleton, BlockSkeleton } from '@/components/states'
import { RankBars, UsageChart } from '@/components/usage-chart'
import { UsageTable } from '@/components/usage-table'
import { UsageRecordSheet } from '@/components/usage-record-sheet'
import { QuickStart } from '@/components/quickstart'
import { api } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { compact, multiplier, num } from '@/lib/format'
import type { UsagePoint, UsageRecord, UsageTotals } from '@/lib/types'

interface Overview {
  balance: number
  multiplier: number
  today: UsageTotals
  week: UsageTotals
  series: UsagePoint[]
  recent: UsageRecord[]
  keys_total: number
  keys_enabled: number
  top_models: { model: string; charged_tokens: number }[]
  announcement: string
}

export default function ConsoleOverview() {
  const { me, refresh } = useAuth()
  const q = useQuery({
    queryKey: ['user', 'overview'],
    queryFn: async () => {
      const data = await api<Overview>('/api/user/overview')
      void refresh()
      return data
    },
  })
  const [selected, setSelected] = useState<UsageRecord | null>(null)
  const d = q.data
  const low = d && d.balance <= 0

  return (
    <>
      <PageHeader
        title={`你好，${me?.nickname || '欢迎回来'}`}
        description="余额、近期消耗与最近的请求。余额以计费 Token 计算。"
        actions={
          <>
            <Button variant="outline" asChild>
              <Link to="/console/redeem">
                <Ticket /> 兑换充值
              </Link>
            </Button>
            <Button asChild>
              <Link to="/console/keys?new=1">
                <Plus /> 创建密钥
              </Link>
            </Button>
          </>
        }
      />
      {d?.announcement && (
        <div className="mb-4 flex items-start gap-2.5 rounded-lg border border-primary/20 bg-primary/[0.07] px-3.5 py-2.5 text-[13px] text-[#c4c8fa]">
          <Megaphone className="mt-0.5 size-4 shrink-0" />
          <p className="whitespace-pre-wrap">{d.announcement}</p>
        </div>
      )}
      {low && (
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3 rounded-lg border border-warning/25 bg-warning/[0.07] px-3.5 py-2.5 text-[13px] text-warning">
          <span>余额已用完，使用你的密钥的请求会返回 402 余额不足。</span>
          <Button size="sm" variant="outline" asChild>
            <Link to="/console/redeem">去兑换</Link>
          </Button>
        </div>
      )}
      {q.isError ? (
        <Section>
          <ErrorState error={q.error} onRetry={() => q.refetch()} />
        </Section>
      ) : (
        <div className="space-y-4">
          <StatStrip
            loading={q.isLoading}
            items={[
              { label: '账户余额', icon: <Wallet />, value: num(d?.balance), tone: low ? 'danger' : 'default', hint: `用户倍率 ${multiplier(d?.multiplier ?? 1)}` },
              { label: '今日消耗', icon: <Activity />, value: compact(d?.today.charged_tokens), hint: `${num(d?.today.requests)} 次请求` },
              { label: '近 7 天消耗', icon: <CalendarDays />, value: compact(d?.week.charged_tokens), hint: `${num(d?.week.requests)} 次请求 · 成功 ${num(d?.week.success_requests)}` },
              { label: 'API 密钥', icon: <KeyRound />, value: `${d?.keys_enabled ?? 0} / ${d?.keys_total ?? 0}`, hint: '启用 / 全部' },
            ]}
          />
          <div className="grid gap-4 lg:grid-cols-3">
            <Section title="近 14 天消耗" description="按天汇总的计费 Token" className="lg:col-span-2">
              <div className="px-2 pt-3 pb-2">{q.isLoading ? <BlockSkeleton className="h-[220px]" /> : <UsageChart points={d?.series || []} />}</div>
            </Section>
            <Section title="模型分布" description="近 7 天计费 Token">
              {q.isLoading ? (
                <div className="p-4">
                  <BlockSkeleton className="h-32" />
                </div>
              ) : (
                <RankBars items={(d?.top_models || []).map((m) => ({ label: m.model || '未知', value: m.charged_tokens }))} empty="近 7 天还没有消耗" format={compact} />
              )}
            </Section>
          </div>
          <Section
            title="最近请求"
            actions={
              d && d.recent.length > 0 ? (
                <Button variant="ghost" size="sm" asChild>
                  <Link to="/console/usage">查看全部</Link>
                </Button>
              ) : undefined
            }
          >
            {q.isLoading ? (
              <TableSkeleton rows={4} cols={6} />
            ) : d && d.recent.length > 0 ? (
              <UsageTable records={d.recent} onSelect={setSelected} />
            ) : (
              <div className="grid gap-6 p-5 lg:grid-cols-[1fr_1.4fr] lg:items-center">
                <EmptyState
                  className="py-4"
                  icon={<Activity />}
                  title="还没有请求"
                  description={d?.keys_total ? '用你的密钥调用 OpenAI 兼容接口，请求会实时出现在这里。' : '先创建一个 API 密钥，然后用右侧示例发起第一个请求。'}
                  action={
                    !d?.keys_total && (
                      <Button size="sm" asChild>
                        <Link to="/console/keys?new=1">
                          <Plus /> 创建密钥
                        </Link>
                      </Button>
                    )
                  }
                />
                <QuickStart />
              </div>
            )}
          </Section>
        </div>
      )}
      <UsageRecordSheet record={selected} onOpenChange={(o) => !o && setSelected(null)} />
    </>
  )
}
