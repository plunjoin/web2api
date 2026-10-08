import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { CheckCircle2, Ticket } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { PageHeader, Section } from '@/components/page'
import { EmptyState, ErrorState, TableSkeleton } from '@/components/states'
import { ApiError, api } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { dateTime, num } from '@/lib/format'

interface HistoryItem {
  code: string
  amount: number
  redeemed_at: number
  note: string
}

export default function ConsoleRedeem() {
  const qc = useQueryClient()
  const { me, refresh } = useAuth()
  const [code, setCode] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [last, setLast] = useState<{ amount: number; balance: number } | null>(null)
  const history = useQuery({ queryKey: ['user', 'redeem-history'], queryFn: () => api<{ history: HistoryItem[]; total: number }>('/api/user/redeem') })

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    const value = code.trim()
    if (!value) return
    setBusy(true)
    setError('')
    try {
      const res = await api<{ amount: number; balance: number }>('/api/user/redeem', { method: 'POST', json: { code: value } })
      setLast(res)
      setCode('')
      toast.success(`兑换成功，到账 ${num(res.amount)} Token`)
      await refresh()
      qc.invalidateQueries({ queryKey: ['user'] })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '兑换失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <PageHeader title="兑换充值" description="输入管理员发放的兑换码，额度会立即计入账户余额。每个兑换码只能使用一次。" />
      <div className="grid gap-4 lg:grid-cols-[minmax(0,420px)_1fr]">
        <Section title="兑换码">
          <form onSubmit={submit} className="space-y-3 p-4">
            <Input
              autoFocus
              value={code}
              onChange={(e) => {
                setCode(e.target.value.toUpperCase())
                setError('')
              }}
              placeholder="W2A-XXXX-XXXX-XXXX-XXXX"
              className="h-10 font-mono text-[14px] tracking-wider"
              aria-invalid={!!error}
              spellCheck={false}
              autoComplete="off"
            />
            {error && <p className="text-xs text-[#ff7a7e]">{error}</p>}
            <Button type="submit" className="w-full" disabled={busy || !code.trim()}>
              <Ticket /> {busy ? '兑换中…' : '兑换'}
            </Button>
            {last && (
              <div className="flex items-start gap-2.5 rounded-md border border-success/20 bg-success/[0.07] p-3 text-[13px]">
                <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-success" />
                <div>
                  <div className="font-medium text-success">到账 +{num(last.amount)} Token</div>
                  <div className="text-xs text-muted-foreground">当前余额 {num(last.balance)}</div>
                </div>
              </div>
            )}
            <div className="flex items-center justify-between border-t pt-3 text-xs text-muted-foreground">
              <span>当前余额</span>
              <span className="text-sm font-medium text-foreground tabular-nums">{num(me?.balance)}</span>
            </div>
          </form>
        </Section>
        <Section title="兑换记录" description={history.data ? `共 ${history.data.total} 次` : undefined}>
          {history.isLoading ? (
            <TableSkeleton rows={3} cols={3} />
          ) : history.isError ? (
            <ErrorState error={history.error} onRetry={() => history.refetch()} />
          ) : !history.data?.history.length ? (
            <EmptyState icon={<Ticket />} title="还没有兑换过" description="兑换成功的记录会显示在这里，兑换码会部分隐藏。" />
          ) : (
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead>兑换码</TableHead>
                  <TableHead className="text-right">额度</TableHead>
                  <TableHead>时间</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {history.data.history.map((h) => (
                  <TableRow key={h.code + h.redeemed_at}>
                    <TableCell className="font-mono text-xs">{h.code}</TableCell>
                    <TableCell className="text-right font-medium text-success tabular-nums">+{num(h.amount)}</TableCell>
                    <TableCell className="text-muted-foreground">{dateTime(h.redeemed_at)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </Section>
      </div>
    </>
  )
}
