import { useMemo, useState } from 'react'
import { keepPreviousData, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Activity, Download, Undo2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { PageHeader, Section, Toolbar } from '@/components/page'
import { StatStrip } from '@/components/stat'
import { EmptyState, ErrorState, TableSkeleton, BlockSkeleton } from '@/components/states'
import { UsageChart, type UsageMetric } from '@/components/usage-chart'
import { UsageTable } from '@/components/usage-table'
import { UsageRecordSheet } from '@/components/usage-record-sheet'
import { Pager } from '@/components/pager'
import { Field, MiniSelect, dayOptions } from '@/components/fields'
import { useConfirm } from '@/components/confirm'
import { useDebounced } from '@/hooks/use-misc'
import { api, download, qs } from '@/lib/api'
import { compact, latency, num } from '@/lib/format'
import type { ApiKey, UsagePoint, UsageRecord } from '@/lib/types'

const LIMIT = 50

interface Breakdown {
  key_id: number
  key_name: string
  key_masked: string
  model: string
  requests: number
  success_requests: number
  total_tokens: number
  charged_tokens: number
  estimated_requests: number
  avg_latency_ms: number
}

export default function AdminUsage() {
  const qc = useQueryClient()
  const confirm = useConfirm()
  const [days, setDays] = useState('7')
  const [keyId, setKeyId] = useState('all')
  const [modelInput, setModelInput] = useState('')
  const model = useDebounced(modelInput.trim())
  const [metric, setMetric] = useState<UsageMetric>('charged_tokens')
  const [offset, setOffset] = useState(0)
  const [selected, setSelected] = useState<UsageRecord | null>(null)
  const [refundNote, setRefundNote] = useState('')
  const filter = { days, key_id: keyId, model }

  const keys = useQuery({ queryKey: ['admin', 'keys'], queryFn: () => api<{ keys: ApiKey[] }>('/admin/api/keys') })
  const summary = useQuery({
    queryKey: ['admin', 'usage-summary', filter],
    queryFn: () => api<{ breakdown: Breakdown[]; totals: { requests: number; success_requests: number; total_tokens: number; charged_tokens: number; estimated_requests: number } }>(`/admin/api/usage${qs(filter)}`),
    placeholderData: keepPreviousData,
  })
  const series = useQuery({
    queryKey: ['admin', 'usage-series', filter],
    queryFn: () => api<{ bucket: 'day' | 'hour'; points: UsagePoint[] }>(`/admin/api/usage/timeseries${qs(filter)}`),
    placeholderData: keepPreviousData,
  })
  const records = useQuery({
    queryKey: ['admin', 'usage-records', filter, offset],
    queryFn: () => api<{ total: number; records: UsageRecord[] }>(`/admin/api/usage/records${qs({ ...filter, limit: LIMIT, offset })}`),
    placeholderData: keepPreviousData,
  })
  const t = summary.data?.totals
  const rate = t && t.requests ? (t.success_requests / t.requests) * 100 : null
  const breakdown = useMemo(() => [...(summary.data?.breakdown || [])].sort((a, b) => b.charged_tokens - a.charged_tokens), [summary.data])

  const refund = async () => {
    if (!selected) return
    if (!(await confirm({ title: `退还 ${num(selected.charged_tokens)} Token？`, description: `退回到 ${selected.user_email || '该用户'} 的余额，并写入一条退款流水。每条记录只能退款一次。`, confirmText: '确认退款' }))) return
    try {
      await api(`/admin/api/usage/records/${selected.id}/refund`, { method: 'POST', json: { note: refundNote.trim() } })
      toast.success('已退款')
      setSelected(null)
      qc.invalidateQueries({ queryKey: ['admin'] })
    } catch (e) {
      toast.error((e as Error).message)
    }
  }

  return (
    <>
      <PageHeader
        title="用量"
        description="全部请求的 Token 与扣费。点击记录查看计算过程；用户密钥的请求可以退款。"
        actions={
          <Button variant="outline" onClick={() => download(`/admin/api/usage/export.csv${qs(filter)}`, 'usage.csv').catch((e) => toast.error(e.message))}>
            <Download /> 导出 CSV
          </Button>
        }
      />
      <Toolbar className="mb-3">
        <MiniSelect value={days} onChange={(v) => { setDays(v); setOffset(0) }} options={dayOptions} />
        <MiniSelect
          value={keyId}
          onChange={(v) => { setKeyId(v); setOffset(0) }}
          className="min-w-[160px]"
          options={[{ value: 'all', label: '全部密钥' }, ...(keys.data?.keys || []).map((k) => ({ value: String(k.id), label: `${k.name || `#${k.id}`}${k.owner_email ? ` · ${k.owner_email}` : ''}` }))]}
        />
        <Input value={modelInput} onChange={(e) => { setModelInput(e.target.value); setOffset(0) }} placeholder="按模型筛选" className="h-7 w-48 text-xs" />
      </Toolbar>
      <div className="space-y-4">
        <StatStrip
          loading={summary.isLoading}
          items={[
            { label: '请求数', value: num(t?.requests) },
            { label: '成功率', value: rate === null ? '—' : `${rate.toFixed(1)}%`, tone: rate !== null && rate < 90 ? 'warning' : 'default' },
            { label: '总 Token', value: compact(t?.total_tokens), hint: t?.estimated_requests ? `${num(t.estimated_requests)} 次为本地估算` : '全部为上游返回' },
            { label: '计费 Token', value: compact(t?.charged_tokens) },
          ]}
        />
        <Section
          title="趋势"
          description={series.data?.bucket === 'hour' ? '按小时' : '按天'}
          actions={
            <Tabs value={metric} onValueChange={(v) => setMetric(v as UsageMetric)}>
              <TabsList className="h-7">
                <TabsTrigger value="charged_tokens" className="px-2 text-xs">计费 Token</TabsTrigger>
                <TabsTrigger value="total_tokens" className="px-2 text-xs">总 Token</TabsTrigger>
                <TabsTrigger value="requests" className="px-2 text-xs">请求</TabsTrigger>
              </TabsList>
            </Tabs>
          }
        >
          <div className="px-2 pt-3 pb-2">
            {series.isLoading ? <BlockSkeleton className="h-[220px]" /> : series.isError ? <ErrorState error={series.error} onRetry={() => series.refetch()} /> : <UsageChart points={series.data?.points || []} bucket={series.data?.bucket} metric={metric} />}
          </div>
        </Section>
        <Section title="按密钥 × 模型" description={`${breakdown.length} 组`}>
          {summary.isLoading ? (
            <TableSkeleton rows={4} cols={6} />
          ) : !breakdown.length ? (
            <EmptyState icon={<Activity />} title="没有数据" description="这个范围内没有请求。" />
          ) : (
            <div className="max-h-[360px] overflow-auto">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead>密钥</TableHead>
                    <TableHead>模型</TableHead>
                    <TableHead className="text-right">请求</TableHead>
                    <TableHead className="text-right">成功</TableHead>
                    <TableHead className="text-right">总 Token</TableHead>
                    <TableHead className="text-right">计费</TableHead>
                    <TableHead className="text-right">平均耗时</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {breakdown.map((r) => (
                    <TableRow key={`${r.key_id}-${r.model}`} className="cursor-pointer" onClick={() => { setKeyId(String(r.key_id)); setModelInput(r.model); setOffset(0) }}>
                      <TableCell className="max-w-[200px] truncate">{r.key_name || r.key_masked}</TableCell>
                      <TableCell className="font-mono text-xs">{r.model || '—'}</TableCell>
                      <TableCell className="text-right tabular-nums">{num(r.requests)}</TableCell>
                      <TableCell className="text-right text-muted-foreground tabular-nums">{num(r.success_requests)}</TableCell>
                      <TableCell className="text-right tabular-nums">{compact(r.total_tokens)}</TableCell>
                      <TableCell className="text-right font-medium tabular-nums">{compact(r.charged_tokens)}</TableCell>
                      <TableCell className="text-right text-muted-foreground tabular-nums">{latency(r.avg_latency_ms)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </Section>
        <Section title="请求记录" description={records.data ? `共 ${num(records.data.total)} 条` : undefined}>
          {records.isLoading ? (
            <TableSkeleton rows={6} cols={8} />
          ) : records.isError ? (
            <ErrorState error={records.error} onRetry={() => records.refetch()} />
          ) : !records.data?.records.length ? (
            <EmptyState icon={<Activity />} title="没有请求记录" />
          ) : (
            <>
              <UsageTable records={records.data.records} onSelect={(r) => { setRefundNote(''); setSelected(r) }} showUser />
              <Pager total={records.data.total} limit={LIMIT} offset={offset} onChange={setOffset} />
            </>
          )}
        </Section>
      </div>
      <UsageRecordSheet
        admin
        record={selected}
        onOpenChange={(o) => !o && setSelected(null)}
        footer={
          selected && selected.user_id > 0 && selected.charged_tokens > 0 && !selected.refunded_tokens ? (
            <div className="space-y-2 rounded-lg border p-3">
              <div className="text-xs font-medium">退款</div>
              <p className="text-[11px] text-muted-foreground">把这次请求扣除的 {num(selected.charged_tokens)} Token 退回用户余额（例如上游故障导致的错误扣费）。</p>
              <Field label="备注（可选）">
                <Input value={refundNote} onChange={(e) => setRefundNote(e.target.value)} placeholder="例如：上游超时" />
              </Field>
              <Button size="sm" variant="outline" onClick={refund}>
                <Undo2 /> 退款
              </Button>
            </div>
          ) : null
        }
      />
    </>
  )
}
