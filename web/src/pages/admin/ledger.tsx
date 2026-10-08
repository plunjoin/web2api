import { useState } from 'react'
import { useSearchParams } from 'react-router'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { ReceiptText, Search, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { PageHeader, Section, Toolbar } from '@/components/page'
import { StatStrip } from '@/components/stat'
import { EmptyState, ErrorState, TableSkeleton } from '@/components/states'
import { Pager } from '@/components/pager'
import { MiniSelect } from '@/components/fields'
import { LedgerTable, ledgerKindOptions } from '@/components/ledger-table'
import { useDebounced } from '@/hooks/use-misc'
import { api, qs } from '@/lib/api'
import { num } from '@/lib/format'
import type { LedgerEntry, User } from '@/lib/types'

const LIMIT = 50
const rangeOptions = [
  { value: '1', label: '近 24 小时' },
  { value: '7', label: '近 7 天' },
  { value: '30', label: '近 30 天' },
  { value: '90', label: '近 90 天' },
  { value: 'all', label: '全部时间' },
]

export default function AdminLedger() {
  const [params, setParams] = useSearchParams()
  const userId = params.get('user_id') || ''
  const [kind, setKind] = useState(params.get('kind') || 'all')
  const [days, setDays] = useState('30')
  const [search, setSearch] = useState('')
  const q = useDebounced(search)
  const [offset, setOffset] = useState(0)

  const user = useQuery({ queryKey: ['admin', 'user', Number(userId)], queryFn: () => api<{ user: User }>(`/admin/api/users/${userId}`), enabled: !!userId })
  const list = useQuery({
    queryKey: ['admin', 'ledger', userId, kind, days, q, offset],
    queryFn: () => api<{ entries: LedgerEntry[]; total: number; sums: { credit: number; debit: number } }>(`/admin/api/ledger${qs({ user_id: userId, kind, days, q, limit: LIMIT, offset })}`),
    placeholderData: keepPreviousData,
  })
  const setUser = (id: number | null) => {
    const next = new URLSearchParams(params)
    if (id) next.set('user_id', String(id))
    else next.delete('user_id')
    setParams(next, { replace: true })
    setOffset(0)
  }
  const d = list.data
  return (
    <>
      <PageHeader title="余额流水" description="所有用户余额变动的不可变记录。每条都带变动前后余额，可逐条对账。" />
      <Toolbar className="mb-3">
        <div className="relative">
          <Search className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input value={search} onChange={(e) => { setSearch(e.target.value); setOffset(0) }} placeholder="搜索邮箱或说明" className="h-7 w-60 pl-8 text-xs" />
        </div>
        <MiniSelect value={days} onChange={(v) => { setDays(v); setOffset(0) }} options={rangeOptions} />
        <MiniSelect value={kind} onChange={(v) => { setKind(v); setOffset(0) }} options={ledgerKindOptions} />
        {userId && (
          <span className="inline-flex h-7 items-center gap-1 rounded-md border bg-white/[0.03] pr-1 pl-2 text-xs">
            用户 {user.data?.user.email || `#${userId}`}
            <Button variant="ghost" size="icon-sm" className="size-5" onClick={() => setUser(null)} aria-label="清除用户筛选">
              <X className="size-3" />
            </Button>
          </span>
        )}
      </Toolbar>
      <div className="space-y-4">
        <StatStrip
          className="lg:grid-cols-3"
          loading={list.isLoading}
          items={[
            { label: '流水条数', value: num(d?.total) },
            { label: '入账', value: `+${num(d?.sums.credit)}`, tone: 'success', hint: '充值、赠送、退款、正向调整' },
            { label: '出账', value: d?.sums.debit ? `-${num(d.sums.debit)}` : '0', hint: '使用扣费、负向调整' },
          ]}
        />
        <Section>
          {list.isLoading ? (
            <TableSkeleton rows={8} cols={7} />
          ) : list.isError ? (
            <ErrorState error={list.error} onRetry={() => list.refetch()} />
          ) : !d?.entries.length ? (
            <EmptyState icon={<ReceiptText />} title="没有符合条件的流水" description="调整时间范围或筛选条件。" />
          ) : (
            <>
              <LedgerTable entries={d.entries} showUser onUserClick={(id) => setUser(id)} />
              <Pager total={d.total} limit={LIMIT} offset={offset} onChange={setOffset} />
            </>
          )}
        </Section>
      </div>
    </>
  )
}
