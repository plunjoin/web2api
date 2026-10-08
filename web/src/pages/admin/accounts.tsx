import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Ban, CheckCircle2, ChevronDown, KeyRound, Plus, RefreshCw, Server, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { PageHeader, Section } from '@/components/page'
import { StatStrip } from '@/components/stat'
import { EmptyState, ErrorState, TableSkeleton } from '@/components/states'
import { RowMenu, type RowAction } from '@/components/row-menu'
import { Pill, StatusDot } from '@/components/status'
import { Field } from '@/components/fields'
import { useConfirm } from '@/components/confirm'
import { api } from '@/lib/api'
import { relative } from '@/lib/format'
import type { Account } from '@/lib/types'

const statusLabel: Record<string, string> = { ok: '正常', ready: '就绪', error: '异常', initializing: '初始化中', unknown: '未知', disabled: '已停用' }

export default function AdminAccounts() {
  const qc = useQueryClient()
  const confirm = useConfirm()
  const q = useQuery({ queryKey: ['admin', 'accounts'], queryFn: () => api<{ accounts: Account[] }>('/admin/api/accounts'), refetchInterval: 15_000 })
  const [adding, setAdding] = useState<'a' | 'b' | null>(null)
  const [creds, setCreds] = useState<Account | null>(null)
  const [checking, setChecking] = useState<number | null>(null)
  const invalidate = () => qc.invalidateQueries({ queryKey: ['admin'] })

  const check = async (a: Account) => {
    setChecking(a.id)
    try {
      await api(`/admin/api/accounts/${a.id}/check`, { method: 'POST' })
      toast.success(`${a.label} 检查通过`)
    } catch (e) {
      toast.error(`${a.label}：${(e as Error).message}`)
    } finally {
      setChecking(null)
      invalidate()
    }
  }
  const actions = (a: Account): RowAction[] => [
    { label: '立即检查', icon: <RefreshCw />, onSelect: () => check(a), disabled: checking === a.id },
    ...(a.engine === 'a' ? [{ label: '更新 Cookie', icon: <KeyRound />, onSelect: () => setCreds(a) }] : []),
    a.enabled
      ? {
          label: '停用',
          icon: <Ban />,
          onSelect: async () => {
            try {
              await api(`/admin/api/accounts/${a.id}`, { method: 'PATCH', json: { enabled: false } })
              toast.success('已停用')
              invalidate()
            } catch (e) {
              toast.error((e as Error).message)
            }
          },
        }
      : {
          label: '启用',
          icon: <CheckCircle2 />,
          onSelect: async () => {
            try {
              await api(`/admin/api/accounts/${a.id}`, { method: 'PATCH', json: { enabled: true } })
              toast.success('已启用')
              invalidate()
            } catch (e) {
              toast.error((e as Error).message)
            }
          },
        },
    {
      label: '删除',
      icon: <Trash2 />,
      destructive: true,
      separatorBefore: true,
      onSelect: async () => {
        if (!(await confirm({ title: `删除账号「${a.label}」？`, description: '账号凭据会被删除，正在进行的请求可能失败。', confirmText: '删除', destructive: true }))) return
        try {
          await api(`/admin/api/accounts/${a.id}`, { method: 'DELETE' })
          toast.success('已删除')
          invalidate()
        } catch (e) {
          toast.error((e as Error).message)
        }
      },
    },
  ]

  const accounts = q.data?.accounts || []
  const ready = accounts.filter((a) => a.enabled && a.live?.ready).length
  const broken = accounts.filter((a) => a.enabled && !a.live?.ready).length
  const addMenu = (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button>
          <Plus /> 添加账号 <ChevronDown className="opacity-60" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-64">
        <DropdownMenuItem onSelect={() => setAdding('a')} className="flex-col items-start gap-0.5">
          <span className="text-[13px]">Gemini 网页号（引擎 A）</span>
          <span className="text-[11px] text-muted-foreground">粘贴 __Secure-1PSID / 1PSIDTS Cookie</span>
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={() => setAdding('b')} className="flex-col items-start gap-0.5">
          <span className="text-[13px]">AI Studio 号（引擎 B）</span>
          <span className="text-[11px] text-muted-foreground">粘贴 Playwright storage_state JSON</span>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )

  return (
    <>
      <PageHeader title="上游账号" description="网关用来访问 Gemini 的账号池。请求在就绪的账号之间轮询，异常账号会自动跳过。" actions={addMenu} />
      <div className="space-y-4">
        <StatStrip
          className="lg:grid-cols-3"
          loading={q.isLoading}
          items={[
            { label: '账号总数', value: accounts.length, hint: `网页 ${accounts.filter((a) => a.engine === 'a').length} · AI Studio ${accounts.filter((a) => a.engine === 'b').length}` },
            { label: '就绪', value: ready, tone: 'success' },
            { label: '异常', value: broken, tone: broken ? 'danger' : 'default', hint: broken ? '启用但未就绪，检查 Cookie 是否过期' : '全部正常' },
          ]}
        />
        <Section>
          {q.isLoading ? (
            <TableSkeleton rows={4} cols={5} />
          ) : q.isError ? (
            <ErrorState error={q.error} onRetry={() => q.refetch()} />
          ) : accounts.length === 0 ? (
            <EmptyState icon={<Server />} title="还没有上游账号" description="至少添加一个账号，网关才能处理请求。" action={addMenu} />
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead>账号</TableHead>
                    <TableHead>引擎</TableHead>
                    <TableHead>状态</TableHead>
                    <TableHead className="text-right">模型数</TableHead>
                    <TableHead>更新时间</TableHead>
                    <TableHead className="w-10" />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {accounts.map((a) => {
                    const st = a.live?.status || a.status
                    const detail = a.live?.detail || a.detail
                    return (
                      <TableRow key={a.id} className="cursor-pointer" onClick={() => check(a)}>
                        <TableCell>
                          <div className="font-medium">{a.label}</div>
                          <div className="text-[11px] text-muted-foreground">#{a.id}</div>
                        </TableCell>
                        <TableCell>
                          <Pill tone={a.engine === 'a' ? 'primary' : 'muted'}>{a.engine === 'a' ? 'Gemini 网页' : 'AI Studio'}</Pill>
                        </TableCell>
                        <TableCell className="max-w-[320px]">
                          {checking === a.id ? (
                            <StatusDot tone="warning">检查中…</StatusDot>
                          ) : !a.enabled ? (
                            <StatusDot tone="muted">已停用</StatusDot>
                          ) : a.live?.ready ? (
                            <StatusDot tone="success">就绪</StatusDot>
                          ) : (
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <span>
                                  <StatusDot tone="danger">{statusLabel[st] || st}</StatusDot>
                                </span>
                              </TooltipTrigger>
                              {detail && <TooltipContent className="max-w-sm">{detail}</TooltipContent>}
                            </Tooltip>
                          )}
                          {detail && a.enabled && !a.live?.ready && <div className="truncate text-[11px] text-muted-foreground">{detail}</div>}
                        </TableCell>
                        <TableCell className="text-right tabular-nums">{a.live?.models ?? 0}</TableCell>
                        <TableCell className="text-muted-foreground">{relative(a.updated_at)}</TableCell>
                        <TableCell onClick={(e) => e.stopPropagation()}>
                          <RowMenu actions={actions(a)} />
                        </TableCell>
                      </TableRow>
                    )
                  })}
                </TableBody>
              </Table>
            </div>
          )}
        </Section>
        {accounts.length > 0 && <p className="text-xs text-muted-foreground">提示：点击一行即可立即检查该账号。</p>}
      </div>
      <AddGeminiDialog open={adding === 'a'} onOpenChange={(o) => !o && setAdding(null)} onDone={invalidate} />
      <AddStudioDialog open={adding === 'b'} onOpenChange={(o) => !o && setAdding(null)} onDone={invalidate} />
      <CredsDialog account={creds} onOpenChange={(o) => !o && setCreds(null)} onDone={invalidate} />
    </>
  )
}

function useReset<T>(open: boolean, initial: T): [T, (v: T) => void] {
  const [v, setV] = useState(initial)
  const [last, setLast] = useState(false)
  if (open !== last) {
    setLast(open)
    if (open) setV(initial)
  }
  return [v, setV]
}

function AddGeminiDialog({ open, onOpenChange, onDone }: { open: boolean; onOpenChange: (o: boolean) => void; onDone: () => void }) {
  const [f, setF] = useReset(open, { label: '', psid: '', psidts: '' })
  const [busy, setBusy] = useState(false)
  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      const storage_state = JSON.stringify({
        cookies: [
          { name: '__Secure-1PSID', value: f.psid.trim(), domain: '.google.com', path: '/', secure: true },
          { name: '__Secure-1PSIDTS', value: f.psidts.trim(), domain: '.google.com', path: '/', secure: true },
        ],
      })
      await api('/admin/api/accounts', { method: 'POST', json: { engine: 'a', label: f.label.trim(), storage_state } })
      toast.success('账号已添加，正在初始化')
      onOpenChange(false)
      onDone()
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
            <DialogTitle className="text-[15px]">添加 Gemini 网页号</DialogTitle>
            <DialogDescription className="text-[13px]">在已登录 gemini.google.com 的浏览器开发者工具 → Application → Cookies 中复制。</DialogDescription>
          </DialogHeader>
          <Field label="名称" htmlFor="ga-label">
            <Input id="ga-label" required autoFocus value={f.label} onChange={(e) => setF({ ...f, label: e.target.value })} placeholder="例如 主号-1" />
          </Field>
          <Field label="__Secure-1PSID" htmlFor="ga-psid">
            <Input id="ga-psid" required value={f.psid} onChange={(e) => setF({ ...f, psid: e.target.value })} className="font-mono" autoComplete="off" spellCheck={false} />
          </Field>
          <Field label="__Secure-1PSIDTS" htmlFor="ga-psidts">
            <Input id="ga-psidts" required value={f.psidts} onChange={(e) => setF({ ...f, psidts: e.target.value })} className="font-mono" autoComplete="off" spellCheck={false} />
          </Field>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button type="submit" disabled={busy}>
              添加
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function AddStudioDialog({ open, onOpenChange, onDone }: { open: boolean; onOpenChange: (o: boolean) => void; onDone: () => void }) {
  const [f, setF] = useReset(open, { email: '', storage_state: '', locale: '', timezone: '' })
  const [busy, setBusy] = useState(false)
  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    try {
      JSON.parse(f.storage_state)
    } catch {
      toast.error('storage_state 不是有效的 JSON')
      return
    }
    setBusy(true)
    try {
      await api('/admin/api/accounts', { method: 'POST', json: { engine: 'b', ...f } })
      toast.success('账号已添加，正在初始化')
      onOpenChange(false)
      onDone()
    } catch (err) {
      toast.error((err as Error).message)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[520px]">
        <form onSubmit={submit} className="space-y-4">
          <DialogHeader>
            <DialogTitle className="text-[15px]">添加 AI Studio 号</DialogTitle>
            <DialogDescription className="text-[13px]">粘贴 Playwright storage_state JSON（需包含 google.com 的登录 Cookie）。</DialogDescription>
          </DialogHeader>
          <Field label="Google 邮箱" htmlFor="gb-email">
            <Input id="gb-email" required autoFocus value={f.email} onChange={(e) => setF({ ...f, email: e.target.value })} placeholder="name@gmail.com" />
          </Field>
          <Field label="storage_state" htmlFor="gb-state">
            <Textarea
              id="gb-state"
              required
              rows={6}
              spellCheck={false}
              value={f.storage_state}
              onChange={(e) => setF({ ...f, storage_state: e.target.value })}
              className="font-mono text-[11px]"
              placeholder='{"cookies":[{"name":"...","value":"...","domain":".google.com","path":"/"}],"origins":[]}'
            />
          </Field>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="locale（可选）" htmlFor="gb-locale">
              <Input id="gb-locale" value={f.locale} onChange={(e) => setF({ ...f, locale: e.target.value })} placeholder="en-US" />
            </Field>
            <Field label="timezone（可选）" htmlFor="gb-tz">
              <Input id="gb-tz" value={f.timezone} onChange={(e) => setF({ ...f, timezone: e.target.value })} placeholder="America/New_York" />
            </Field>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button type="submit" disabled={busy}>
              添加
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function CredsDialog({ account, onOpenChange, onDone }: { account: Account | null; onOpenChange: (o: boolean) => void; onDone: () => void }) {
  const [f, setF] = useReset(!!account, { psid: '', psidts: '' })
  const [busy, setBusy] = useState(false)
  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!account) return
    setBusy(true)
    try {
      await api(`/admin/api/accounts/${account.id}/credentials`, { method: 'PUT', json: { psid: f.psid.trim(), psidts: f.psidts.trim() } })
      toast.success('Cookie 已更新')
      onOpenChange(false)
      onDone()
    } catch (err) {
      toast.error((err as Error).message)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Dialog open={!!account} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[460px]">
        <form onSubmit={submit} className="space-y-4">
          <DialogHeader>
            <DialogTitle className="text-[15px]">更新 Cookie</DialogTitle>
            <DialogDescription className="text-[13px]">{account?.label} · 旧 Cookie 会被替换。</DialogDescription>
          </DialogHeader>
          <Field label="__Secure-1PSID" htmlFor="c-psid">
            <Input id="c-psid" required autoFocus value={f.psid} onChange={(e) => setF({ ...f, psid: e.target.value })} className="font-mono" autoComplete="off" spellCheck={false} />
          </Field>
          <Field label="__Secure-1PSIDTS" htmlFor="c-psidts">
            <Input id="c-psidts" required value={f.psidts} onChange={(e) => setF({ ...f, psidts: e.target.value })} className="font-mono" autoComplete="off" spellCheck={false} />
          </Field>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button type="submit" disabled={busy}>
              保存
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
