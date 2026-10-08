import { useState } from 'react'
import { useSearchParams } from 'react-router'
import { keepPreviousData, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Ban, CheckCircle2, Copy, Download, Layers, Plus, Search, Ticket, Trash2, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { PageHeader, Section, Toolbar } from '@/components/page'
import { StatStrip } from '@/components/stat'
import { EmptyState, ErrorState, TableSkeleton } from '@/components/states'
import { RowMenu, type RowAction } from '@/components/row-menu'
import { Pill } from '@/components/status'
import { Pager } from '@/components/pager'
import { Field, MiniSelect } from '@/components/fields'
import { copyText } from '@/components/copy'
import { useConfirm } from '@/components/confirm'
import { useDebounced, useNewParam } from '@/hooks/use-misc'
import { api, download, qs } from '@/lib/api'
import { codeStatusLabel, compact, dateOnly, dateTime, num, relative } from '@/lib/format'
import type { CodeStats, RedeemCode } from '@/lib/types'

const LIMIT = 50
const statusTone = { unused: 'success', redeemed: 'primary', disabled: 'muted', expired: 'warning' } as const

export default function AdminCodes() {
  const qc = useQueryClient()
  const confirm = useConfirm()
  const [params, setParams] = useSearchParams()
  const batch = params.get('batch') || ''
  const [search, setSearch] = useState('')
  const q = useDebounced(search)
  const [status, setStatus] = useState('all')
  const [offset, setOffset] = useState(0)
  const [genOpen, setGenOpen] = useNewParam()
  const [result, setResult] = useState<{ codes: RedeemCode[]; batch: string } | null>(null)

  const filter = { status, batch, q }
  const list = useQuery({
    queryKey: ['admin', 'codes', filter, offset],
    queryFn: () => api<{ codes: RedeemCode[]; total: number; stats: CodeStats }>(`/admin/api/redeem-codes${qs({ ...filter, limit: LIMIT, offset })}`),
    placeholderData: keepPreviousData,
  })
  const invalidate = () => qc.invalidateQueries({ queryKey: ['admin'] })
  const setBatch = (b: string) => {
    const next = new URLSearchParams(params)
    if (b) next.set('batch', b)
    else next.delete('batch')
    setParams(next, { replace: true })
    setOffset(0)
  }

  const run = async (fn: () => Promise<unknown>, ok: string) => {
    try {
      await fn()
      toast.success(ok)
      invalidate()
    } catch (e) {
      toast.error((e as Error).message)
    }
  }
  const disableBatch = async (b: string) => {
    if (!(await confirm({ title: `停用批次 ${b} 中所有未使用的兑换码？`, description: '已兑换的不受影响。停用后可以逐个重新启用。', confirmText: '停用整批', destructive: true }))) return
    try {
      const res = await api<{ disabled: number }>(`/admin/api/redeem-codes/batches/${encodeURIComponent(b)}/disable`, { method: 'POST' })
      toast.success(`已停用 ${res.disabled} 个兑换码`)
      invalidate()
    } catch (e) {
      toast.error((e as Error).message)
    }
  }
  const actions = (c: RedeemCode): RowAction[] => {
    const items: RowAction[] = [{ label: '复制兑换码', icon: <Copy />, onSelect: () => copyText(c.code, '兑换码已复制') }]
    if (c.status !== 'redeemed') {
      items.push(
        c.enabled
          ? { label: '停用', icon: <Ban />, onSelect: () => run(() => api(`/admin/api/redeem-codes/${c.id}`, { method: 'PATCH', json: { enabled: false } }), '已停用') }
          : { label: '启用', icon: <CheckCircle2 />, onSelect: () => run(() => api(`/admin/api/redeem-codes/${c.id}`, { method: 'PATCH', json: { enabled: true } }), '已启用') },
      )
    }
    items.push({ label: `只看批次 ${c.batch}`, icon: <Layers />, onSelect: () => setBatch(c.batch) })
    items.push({ label: '停用整个批次', icon: <Ban />, onSelect: () => disableBatch(c.batch) })
    if (c.status !== 'redeemed') {
      items.push({
        label: '删除',
        icon: <Trash2 />,
        destructive: true,
        separatorBefore: true,
        onSelect: async () => {
          if (!(await confirm({ title: '删除这个兑换码？', description: '删除后无法恢复。已兑换的码不能删除（作为充值凭据保留）。', confirmText: '删除', destructive: true }))) return
          run(() => api(`/admin/api/redeem-codes/${c.id}`, { method: 'DELETE' }), '已删除')
        },
      })
    }
    return items
  }

  const s = list.data?.stats
  const codes = list.data?.codes || []
  const filtered = !!q || status !== 'all' || !!batch
  return (
    <>
      <PageHeader
        title="兑换码"
        description="批量生成充值码分发给用户。每个码只能兑换一次，兑换人和时间都会记录。"
        actions={
          <>
            <Button variant="outline" onClick={() => download(`/admin/api/redeem-codes/export.csv${qs(filter)}`, 'redeem-codes.csv').catch((e) => toast.error(e.message))}>
              <Download /> 导出 CSV
            </Button>
            <Button onClick={() => setGenOpen(true)}>
              <Plus /> 生成兑换码
            </Button>
          </>
        }
      />
      <div className="space-y-4">
        <StatStrip
          className="lg:grid-cols-3"
          loading={list.isLoading}
          items={[
            { label: '全部兑换码', value: num(s?.total) },
            { label: '可用', value: num(s?.unused), tone: 'success', hint: `面值合计 ${compact(s?.unused_amount)} Token` },
            { label: '已兑换', value: num(s?.redeemed), hint: `面值合计 ${compact(s?.redeemed_amount)} Token` },
          ]}
        />
        <Toolbar>
          <div className="relative">
            <Search className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input value={search} onChange={(e) => { setSearch(e.target.value); setOffset(0) }} placeholder="搜索兑换码、备注或兑换人" className="h-7 w-64 pl-8 text-xs" />
          </div>
          <MiniSelect
            value={status}
            onChange={(v) => { setStatus(v); setOffset(0) }}
            options={[{ value: 'all', label: '全部状态' }, { value: 'unused', label: '可用' }, { value: 'redeemed', label: '已兑换' }, { value: 'disabled', label: '已停用' }, { value: 'expired', label: '已过期' }]}
          />
          {batch && (
            <span className="inline-flex h-7 items-center gap-1 rounded-md border bg-white/[0.03] pr-1 pl-2 text-xs">
              批次 <span className="font-mono">{batch}</span>
              <Button variant="ghost" size="icon-sm" className="size-5" onClick={() => setBatch('')} aria-label="清除批次筛选">
                <X className="size-3" />
              </Button>
            </span>
          )}
          {batch && (
            <Button variant="outline" size="sm" className="h-7 text-xs" onClick={() => disableBatch(batch)}>
              <Ban /> 停用本批次
            </Button>
          )}
        </Toolbar>
        <Section title={list.data ? `${num(list.data.total)} 个兑换码` : '兑换码'}>
          {list.isLoading ? (
            <TableSkeleton rows={6} cols={6} />
          ) : list.isError ? (
            <ErrorState error={list.error} onRetry={() => list.refetch()} />
          ) : codes.length === 0 ? (
            filtered ? (
              <EmptyState icon={<Search />} title="没有匹配的兑换码" description="换个条件试试。" />
            ) : (
              <EmptyState
                icon={<Ticket />}
                title="还没有兑换码"
                description="生成一批兑换码，复制或导出后发给用户，用户在控制台的“兑换充值”页使用。"
                action={
                  <Button size="sm" onClick={() => setGenOpen(true)}>
                    <Plus /> 生成兑换码
                  </Button>
                }
              />
            )
          ) : (
            <>
              <div className="overflow-x-auto">
                <Table>
                  <TableHeader>
                    <TableRow className="hover:bg-transparent">
                      <TableHead>兑换码</TableHead>
                      <TableHead className="text-right">面值</TableHead>
                      <TableHead>状态</TableHead>
                      <TableHead>批次</TableHead>
                      <TableHead>兑换人</TableHead>
                      <TableHead>有效期</TableHead>
                      <TableHead>创建</TableHead>
                      <TableHead className="w-10" />
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {codes.map((c) => (
                      <TableRow key={c.id} className="cursor-pointer" onClick={() => copyText(c.code, '兑换码已复制')}>
                        <TableCell>
                          <div className="font-mono text-xs">{c.code}</div>
                          {c.note && <div className="max-w-[260px] truncate text-[11px] text-muted-foreground">{c.note}</div>}
                        </TableCell>
                        <TableCell className="text-right font-medium tabular-nums">{num(c.amount)}</TableCell>
                        <TableCell>
                          <Pill tone={statusTone[c.status]}>{codeStatusLabel[c.status]}</Pill>
                        </TableCell>
                        <TableCell onClick={(e) => e.stopPropagation()}>
                          <button className="font-mono text-xs text-muted-foreground hover:text-primary hover:underline" onClick={() => setBatch(c.batch)}>
                            {c.batch}
                          </button>
                        </TableCell>
                        <TableCell className="max-w-[200px]">
                          {c.redeemed_at ? (
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <div className="truncate">
                                  {c.redeemed_email || `#${c.redeemed_by}`}
                                  <div className="text-[11px] text-muted-foreground">{relative(c.redeemed_at)}</div>
                                </div>
                              </TooltipTrigger>
                              <TooltipContent>{dateTime(c.redeemed_at)}</TooltipContent>
                            </Tooltip>
                          ) : (
                            <span className="text-muted-foreground">—</span>
                          )}
                        </TableCell>
                        <TableCell className="text-muted-foreground">{c.expires_at ? dateOnly(c.expires_at) : '长期'}</TableCell>
                        <TableCell className="text-muted-foreground">
                          {relative(c.created_at)}
                          <div className="text-[11px]">{c.created_by}</div>
                        </TableCell>
                        <TableCell onClick={(e) => e.stopPropagation()}>
                          <RowMenu actions={actions(c)} />
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
              <Pager total={list.data!.total} limit={LIMIT} offset={offset} onChange={setOffset} />
            </>
          )}
        </Section>
      </div>
      <GenerateDialog
        open={genOpen}
        onOpenChange={setGenOpen}
        onCreated={(r) => {
          setResult(r)
          invalidate()
        }}
      />
      <ResultDialog result={result} onClose={() => setResult(null)} onViewBatch={(b) => { setResult(null); setBatch(b) }} />
    </>
  )
}

function GenerateDialog({ open, onOpenChange, onCreated }: { open: boolean; onOpenChange: (o: boolean) => void; onCreated: (r: { codes: RedeemCode[]; batch: string }) => void }) {
  const [f, setF] = useState({ amount: '', count: '10', batch: '', note: '', expireDays: '' })
  const [busy, setBusy] = useState(false)
  const [last, setLast] = useState(false)
  if (open !== last) {
    setLast(open)
    if (open) setF({ amount: '', count: '10', batch: '', note: '', expireDays: '' })
  }
  const amount = Number(f.amount) || 0
  const count = Number(f.count) || 0
  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      const days = Number(f.expireDays) || 0
      const expires_at = days > 0 ? Math.floor(Date.now() / 1000) + days * 86400 : 0
      const res = await api<{ codes: RedeemCode[]; batch: string }>('/admin/api/redeem-codes', {
        method: 'POST',
        json: { amount, count, batch: f.batch.trim(), note: f.note.trim(), expires_at },
      })
      toast.success(`已生成 ${res.codes.length} 个兑换码`)
      onOpenChange(false)
      onCreated(res)
    } catch (err) {
      toast.error((err as Error).message)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[460px]">
        <form onSubmit={submit} className="space-y-4">
          <DialogHeader>
            <DialogTitle className="text-[15px]">生成兑换码</DialogTitle>
            <DialogDescription className="text-[13px]">同一次生成的兑换码属于同一批次，便于整批导出或停用。</DialogDescription>
          </DialogHeader>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="面值（Token）" htmlFor="g-amount">
              <Input id="g-amount" autoFocus required inputMode="numeric" value={f.amount} onChange={(e) => setF({ ...f, amount: e.target.value.replace(/[^\d]/g, '') })} placeholder="例如 1000000" />
            </Field>
            <Field label="数量" htmlFor="g-count" hint="1–1000">
              <Input id="g-count" required inputMode="numeric" value={f.count} onChange={(e) => setF({ ...f, count: e.target.value.replace(/[^\d]/g, '') })} />
            </Field>
            <Field label="批次名" htmlFor="g-batch" hint="留空自动生成">
              <Input id="g-batch" value={f.batch} maxLength={40} onChange={(e) => setF({ ...f, batch: e.target.value })} placeholder="例如 2026-国庆活动" />
            </Field>
            <Field label="有效天数" htmlFor="g-exp" hint="留空表示长期有效">
              <Input id="g-exp" inputMode="numeric" value={f.expireDays} onChange={(e) => setF({ ...f, expireDays: e.target.value.replace(/[^\d]/g, '') })} placeholder="长期" />
            </Field>
            <Field label="备注" htmlFor="g-note" className="sm:col-span-2">
              <Input id="g-note" value={f.note} maxLength={200} onChange={(e) => setF({ ...f, note: e.target.value })} placeholder="仅管理员可见，例如发放渠道" />
            </Field>
          </div>
          {amount > 0 && count > 0 && (
            <p className="rounded-md border bg-white/[0.02] px-3 py-2 text-xs text-muted-foreground">
              将生成 <span className="text-foreground">{num(count)}</span> 个兑换码，总面值 <span className="text-foreground tabular-nums">{num(amount * count)}</span> Token
            </p>
          )}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button type="submit" disabled={busy || !amount || !count || count > 1000}>
              生成
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function ResultDialog({ result, onClose, onViewBatch }: { result: { codes: RedeemCode[]; batch: string } | null; onClose: () => void; onViewBatch: (b: string) => void }) {
  const text = result?.codes.map((c) => c.code).join('\n') || ''
  const downloadCsv = () => {
    if (!result) return
    const rows = ['code,amount,batch,expires_at', ...result.codes.map((c) => `${c.code},${c.amount},${c.batch},${c.expires_at ? new Date(c.expires_at * 1000).toISOString() : ''}`)]
    const blob = new Blob(['\uFEFF' + rows.join('\n')], { type: 'text/csv;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `redeem-codes-${result.batch}.csv`
    a.click()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  }
  return (
    <Dialog open={!!result} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-[520px]">
        <DialogHeader>
          <DialogTitle className="text-[15px]">已生成 {result?.codes.length} 个兑换码</DialogTitle>
          <DialogDescription className="text-[13px]">
            批次 <span className="font-mono text-foreground">{result?.batch}</span>，每个面值 {num(result?.codes[0]?.amount)} Token。之后也可以在列表中按批次导出。
          </DialogDescription>
        </DialogHeader>
        <Textarea readOnly value={text} rows={Math.min(10, (result?.codes.length || 1) + 1)} className="font-mono text-xs" onFocus={(e) => e.currentTarget.select()} />
        <DialogFooter className="gap-2 sm:justify-between">
          <Button variant="ghost" onClick={() => result && onViewBatch(result.batch)}>
            <Layers /> 查看该批次
          </Button>
          <div className="flex gap-2">
            <Button variant="outline" onClick={downloadCsv}>
              <Download /> 下载 CSV
            </Button>
            <Button onClick={() => copyText(text, `已复制 ${result?.codes.length} 个兑换码`)}>
              <Copy /> 全部复制
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
