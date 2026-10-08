import { useState } from 'react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Activity } from 'lucide-react'
import { PageHeader, Section, Toolbar } from '@/components/page'
import { StatStrip } from '@/components/stat'
import { EmptyState, ErrorState, TableSkeleton, BlockSkeleton } from '@/components/states'
import { UsageChart } from '@/components/usage-chart'
import { UsageTable } from '@/components/usage-table'
import { UsageRecordSheet } from '@/components/usage-record-sheet'
import { Pager } from '@/components/pager'
import { MiniSelect, dayOptions } from '@/components/fields'
import { api, qs } from '@/lib/api'
import { compact, num } from '@/lib/format'
import type { ApiKey, UsagePoint, UsageRecord, UsageTotals } from '@/lib/types'

const LIMIT = 50

export default function ConsoleUsage() {
  const [days, setDays] = useState('7')
  const [keyId, setKeyId] = useState('all')
  const [offset, setOffset] = useState(0)
  const [selected, setSelected] = useState<UsageRecord | null>(null)
  const filter = { days, key_id: keyId }

  const keys = useQuery({ queryKey: ['user', 'keys'], queryFn: () => api<{ keys: ApiKey[] }>('/api/user/keys') })
  const series = useQuery({
    queryKey: ['user', 'usage-series', filter],
    queryFn: () => api<{ bucket: 'day' | 'hour'; points: UsagePoint[] }>(`/api/user/usage/timeseries${qs(filter)}`),
  })
  const records = useQuery({
    queryKey: ['user', 'usage-records', filter, offset],
    queryFn: () => api<{ total: number; records: UsageRecord[]; totals: UsageTotals }>(`/api/user/usage/records${qs({ ...filter, limit: LIMIT, offset })}`),
    placeholderData: keepPreviousData,
  })
  const t = records.data?.totals
  const rate = t && t.requests ? `${((t.success_requests / t.requests) * 100).toFixed(1)}%` : '—'

  return (
    <>
      <PageHeader title="用量明细" description="每一次请求的 Token、倍率与扣费。点击一行查看完整计算过程。" />
      <Toolbar className="mb-3">
        <MiniSelect value={days} onChange={(v) => { setDays(v); setOffset(0) }} options={dayOptions} />
        <MiniSelect
          value={keyId}
          onChange={(v) => { setKeyId(v); setOffset(0) }}
          className="min-w-[150px]"
          options={[{ value: 'all', label: '全部密钥' }, ...(keys.data?.keys || []).map((k) => ({ value: String(k.id), label: k.name || `密钥 #${k.id}` }))]}
        />
      </Toolbar>
      <div className="space-y-4">
        <StatStrip
          loading={records.isLoading}
          items={[
            { label: '请求数', value: num(t?.requests) },
            { label: '成功率', value: rate, tone: t && t.requests && t.success_requests / t.requests < 0.9 ? 'warning' : 'default' },
            { label: '总 Token', value: compact(t?.total_tokens) },
            { label: '计费 Token', value: compact(t?.charged_tokens), hint: '已从余额扣除' },
          ]}
        />
        <Section title="消耗趋势" description={series.data?.bucket === 'hour' ? '按小时' : '按天'}>
          <div className="px-2 pt-3 pb-2">
            {series.isLoading ? <BlockSkeleton className="h-[220px]" /> : series.isError ? <ErrorState error={series.error} onRetry={() => series.refetch()} /> : <UsageChart points={series.data?.points || []} bucket={series.data?.bucket} />}
          </div>
        </Section>
        <Section title="请求记录" description={records.data ? `共 ${num(records.data.total)} 条` : undefined}>
          {records.isLoading ? (
            <TableSkeleton rows={6} cols={7} />
          ) : records.isError ? (
            <ErrorState error={records.error} onRetry={() => records.refetch()} />
          ) : !records.data?.records.length ? (
            <EmptyState icon={<Activity />} title="这个时间范围内没有请求" description="换一个时间范围或密钥试试。" />
          ) : (
            <>
              <UsageTable records={records.data.records} onSelect={setSelected} />
              <Pager total={records.data.total} limit={LIMIT} offset={offset} onChange={setOffset} />
            </>
          )}
        </Section>
      </div>
      <UsageRecordSheet record={selected} onOpenChange={(o) => !o && setSelected(null)} />
    </>
  )
}
