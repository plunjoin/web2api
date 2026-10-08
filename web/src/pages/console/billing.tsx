import { useState } from 'react'
import { Link } from 'react-router'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { ReceiptText, Ticket } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { PageHeader, Section, Toolbar } from '@/components/page'
import { StatStrip } from '@/components/stat'
import { EmptyState, ErrorState, TableSkeleton } from '@/components/states'
import { Pager } from '@/components/pager'
import { MiniSelect } from '@/components/fields'
import { LedgerTable, ledgerKindOptions } from '@/components/ledger-table'
import { api, qs } from '@/lib/api'
import { num } from '@/lib/format'
import type { LedgerEntry } from '@/lib/types'

const LIMIT = 50
const rangeOptions = [
  { value: '7', label: '近 7 天' },
  { value: '30', label: '近 30 天' },
  { value: '90', label: '近 90 天' },
  { value: 'all', label: '全部时间' },
]

export default function ConsoleBilling() {
  const [kind, setKind] = useState('all')
  const [days, setDays] = useState('30')
  const [offset, setOffset] = useState(0)
  const q = useQuery({
    queryKey: ['user', 'ledger', kind, days, offset],
    queryFn: () => api<{ entries: LedgerEntry[]; total: number; sums: { credit: number; debit: number }; balance: number }>(`/api/user/ledger${qs({ kind, days, limit: LIMIT, offset })}`),
    placeholderData: keepPreviousData,
  })
  const d = q.data
  return (
    <>
      <PageHeader
        title="账单"
        description="余额的每一次变动都有记录：充值、扣费、退款和调整，并标明变动前后的余额。"
        actions={
          <Button variant="outline" asChild>
            <Link to="/console/redeem">
              <Ticket /> 兑换充值
            </Link>
          </Button>
        }
      />
      <Toolbar className="mb-3">
        <MiniSelect value={days} onChange={(v) => { setDays(v); setOffset(0) }} options={rangeOptions} />
        <MiniSelect value={kind} onChange={(v) => { setKind(v); setOffset(0) }} options={ledgerKindOptions} />
      </Toolbar>
      <div className="space-y-4">
        <StatStrip
          className="lg:grid-cols-3"
          loading={q.isLoading}
          items={[
            { label: '当前余额', value: num(d?.balance), tone: d && d.balance <= 0 ? 'danger' : 'default' },
            { label: '收入', value: `+${num(d?.sums.credit)}`, tone: 'success', hint: '充值、退款、赠送与管理员加款' },
            { label: '支出', value: d?.sums.debit ? `-${num(d.sums.debit)}` : '0', hint: '请求扣费与管理员扣款' },
          ]}
        />
        <Section title="余额流水" description={d ? `共 ${num(d.total)} 条` : undefined}>
          {q.isLoading ? (
            <TableSkeleton rows={6} cols={6} />
          ) : q.isError ? (
            <ErrorState error={q.error} onRetry={() => q.refetch()} />
          ) : !d?.entries.length ? (
            <EmptyState icon={<ReceiptText />} title="没有流水记录" description="充值或调用接口后，余额变动会出现在这里。" />
          ) : (
            <>
              <LedgerTable entries={d.entries} />
              <Pager total={d.total} limit={LIMIT} offset={offset} onChange={setOffset} />
            </>
          )}
        </Section>
      </div>
    </>
  )
}
