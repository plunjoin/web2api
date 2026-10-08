import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Ban, CheckCircle2, Copy, KeyRound, Pencil, Plus, RefreshCw, RotateCcw, Search, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { PageHeader, Section, Toolbar } from '@/components/page'
import { EmptyState, ErrorState, TableSkeleton } from '@/components/states'
import { RowMenu, type RowAction } from '@/components/row-menu'
import { Pill, StatusDot } from '@/components/status'
import { Field, MiniSelect } from '@/components/fields'
import { CopyButton, copyText } from '@/components/copy'
import { useConfirm } from '@/components/confirm'
import { useNewParam } from '@/hooks/use-misc'
import { QuotaBar } from '@/components/quota-bar'
import { api } from '@/lib/api'
import { compact, dateOnly, maskKey, multiplier, num } from '@/lib/format'
import type { ApiKey } from '@/lib/types'

function keyStatus(k: ApiKey) {
  if (!k.enabled) return <StatusDot tone="muted">已停用</StatusDot>
  if (k.expired) return <StatusDot tone="warning">已过期</StatusDot>
  if (k.quota_exhausted) return <StatusDot tone="danger">额度用尽</StatusDot>
  return <StatusDot tone="success">可用</StatusDot>
}

export default function AdminKeys() {
  const qc = useQueryClient()
  const confirm = useConfirm()
  const q = useQuery({ queryKey: ['admin', 'keys'], queryFn: () => api<{ keys: ApiKey[] }>('/admin/api/keys') })
  const [search, setSearch] = useState('')
  const [owner, setOwner] = useState('all')
  const [createOpen, setCreateOpen] = useNewParam()
  const [edit, setEdit] = useState<ApiKey | null>(null)
  const [revealed, setRevealed] = useState<ApiKey | null>(null)
  const invalidate = () => qc.invalidateQueries({ queryKey: ['admin'] })

  const patch = async (k: ApiKey, body: Record<string, unknown>, ok: string) => {
    try {
      await api(`/admin/api/keys/${k.id}`, { method: 'PATCH', json: body })
      toast.success(ok)
      invalidate()
    } catch (e) {
      toast.error((e as Error).message)
    }
  }
  const actions = (k: ApiKey): RowAction[] => [
    { label: '复制密钥', icon: <Copy />, onSelect: () => copyText(k.key, '密钥已复制') },
    { label: '编辑', icon: <Pencil />, onSelect: () => setEdit(k) },
    k.enabled ? { label: '停用', icon: <Ban />, onSelect: () => patch(k, { enabled: false }, '已停用') } : { label: '启用', icon: <CheckCircle2 />, onSelect: () => patch(k, { enabled: true }, '已启用') },
    {
      label: '清零已用额度',
      icon: <RotateCcw />,
      disabled: !k.tokens_used,
      onSelect: async () => {
        if (await confirm({ title: '清零已用额度？', description: '只重置这个密钥的“已用”计数，不影响用量记录和用户余额。', confirmText: '清零' })) patch(k, { reset_usage: true }, '已清零')
      },
    },
    {
      label: '重新生成',
      icon: <RefreshCw />,
      onSelect: async () => {
        if (!(await confirm({ title: '重新生成密钥？', description: '旧密钥立即失效，额度、倍率等设置保留。', confirmText: '重新生成' }))) return
        try {
          const res = await api<{ key: ApiKey }>(`/admin/api/keys/${k.id}/regenerate`, { method: 'POST' })
          setRevealed(res.key)
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
        if (!(await confirm({ title: `删除密钥「${k.name || maskKey(k.key)}」？`, description: k.owner_email ? `该密钥属于用户 ${k.owner_email}。删除后无法恢复。` : '删除后无法恢复，使用它的请求会立即返回 401。', confirmText: '删除', destructive: true }))) return
        try {
          await api(`/admin/api/keys/${k.id}`, { method: 'DELETE' })
          toast.success('已删除')
          invalidate()
        } catch (e) {
          toast.error((e as Error).message)
        }
      },
    },
  ]

  const keys = useMemo(() => {
    const s = search.trim().toLowerCase()
    return (q.data?.keys || []).filter((k) => {
      if (owner === 'admin' && k.user_id) return false
      if (owner === 'user' && !k.user_id) return false
      if (!s) return true
      return [k.name, k.key.slice(-6), k.owner_email].some((v) => v?.toLowerCase().includes(s))
    })
  }, [q.data, search, owner])

  return (
    <>
      <PageHeader
        title="API 密钥"
        description="所有密钥。管理员创建的密钥不属于任何用户、不扣余额；用户自己创建的密钥从其余额扣费。"
        actions={
          <Button onClick={() => setCreateOpen(true)}>
            <Plus /> 创建密钥
          </Button>
        }
      />
      <Toolbar className="mb-3">
        <div className="relative">
          <Search className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="搜索名称、密钥末尾或用户" className="h-7 w-60 pl-8 text-xs" />
        </div>
        <MiniSelect value={owner} onChange={setOwner} options={[{ value: 'all', label: '全部归属' }, { value: 'admin', label: '管理员密钥' }, { value: 'user', label: '用户密钥' }]} />
      </Toolbar>
      <Section title={q.data ? `${keys.length} 个密钥` : '密钥'}>
        {q.isLoading ? (
          <TableSkeleton rows={6} cols={7} />
        ) : q.isError ? (
          <ErrorState error={q.error} onRetry={() => q.refetch()} />
        ) : keys.length === 0 ? (
          <EmptyState
            icon={<KeyRound />}
            title={search || owner !== 'all' ? '没有匹配的密钥' : '还没有密钥'}
            action={
              !search && owner === 'all' ? (
                <Button size="sm" onClick={() => setCreateOpen(true)}>
                  <Plus /> 创建密钥
                </Button>
              ) : undefined
            }
          />
        ) : (
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead>名称</TableHead>
                  <TableHead>归属</TableHead>
                  <TableHead>状态</TableHead>
                  <TableHead>额度</TableHead>
                  <TableHead className="text-right">倍率</TableHead>
                  <TableHead>限制</TableHead>
                  <TableHead className="text-right">24 小时</TableHead>
                  <TableHead className="w-10" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {keys.map((k) => (
                  <TableRow key={k.id} className="cursor-pointer" onClick={() => setEdit(k)}>
                    <TableCell>
                      <div className="font-medium">{k.name || '未命名'}</div>
                      <div className="font-mono text-[11px] text-muted-foreground">{maskKey(k.key)}</div>
                    </TableCell>
                    <TableCell className="max-w-[180px] truncate">{k.owner_email ? k.owner_email : <Pill>管理员</Pill>}</TableCell>
                    <TableCell>{keyStatus(k)}</TableCell>
                    <TableCell>
                      <QuotaBar used={k.tokens_used} limit={k.token_limit} />
                    </TableCell>
                    <TableCell className="text-right tabular-nums">{multiplier(k.multiplier)}</TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      {[k.expires_at ? `${dateOnly(k.expires_at)} 到期` : '', k.rpm_limit ? `${k.rpm_limit} RPM` : '', k.allowed_models?.length ? `${k.allowed_models.length} 个模型` : ''].filter(Boolean).join(' · ') || '—'}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      <div>{compact(k.charged_tokens_24h)}</div>
                      <div className="text-[11px] text-muted-foreground">{num(k.requests_24h)} 次</div>
                    </TableCell>
                    <TableCell onClick={(e) => e.stopPropagation()}>
                      <RowMenu actions={actions(k)} />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </Section>
      <KeyDialog open={createOpen} onOpenChange={setCreateOpen} onSaved={(k) => { setRevealed(k); invalidate() }} />
      <KeyDialog open={!!edit} keyItem={edit} onOpenChange={(o) => !o && setEdit(null)} onSaved={invalidate} />
      <Dialog open={!!revealed} onOpenChange={(o) => !o && setRevealed(null)}>
        <DialogContent className="sm:max-w-[480px]">
          <DialogHeader>
            <DialogTitle className="text-[15px]">密钥已就绪</DialogTitle>
            <DialogDescription className="text-[13px]">复制后配置到客户端；之后在列表中也能复制。</DialogDescription>
          </DialogHeader>
          <div className="flex items-center gap-2 rounded-md border bg-[#0a0a0c] px-3 py-2">
            <code className="flex-1 truncate font-mono text-[12.5px]">{revealed?.key}</code>
            {revealed && <CopyButton value={revealed.key} label="密钥已复制" />}
          </div>
          <DialogFooter>
            <Button onClick={() => setRevealed(null)}>完成</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

function toDateInput(ts: number) {
  if (!ts) return ''
  const d = new Date(ts * 1000)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

function KeyDialog({ open, onOpenChange, keyItem, onSaved }: { open: boolean; onOpenChange: (o: boolean) => void; keyItem?: ApiKey | null; onSaved: (k: ApiKey) => void }) {
  const editing = !!keyItem
  const init = () => ({
    name: keyItem?.name || '',
    limit: keyItem?.token_limit ? String(keyItem.token_limit) : '',
    multiplier: keyItem ? String(keyItem.multiplier) : '1',
    expires: toDateInput(keyItem?.expires_at || 0),
    rpm: keyItem?.rpm_limit ? String(keyItem.rpm_limit) : '',
    models: (keyItem?.allowed_models || []).join('\n'),
  })
  const [f, setF] = useState(init)
  const [busy, setBusy] = useState(false)
  const [last, setLast] = useState(false)
  if (open !== last) {
    setLast(open)
    if (open) setF(init())
  }
  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    const mult = Number(f.multiplier)
    if (!(mult > 0)) {
      toast.error('倍率必须大于 0')
      return
    }
    const expires_at = f.expires ? Math.floor(new Date(`${f.expires}T23:59:59`).getTime() / 1000) : 0
    const body = {
      name: f.name.trim(),
      token_limit: Number(f.limit) || 0,
      multiplier: mult,
      expires_at,
      rpm_limit: Number(f.rpm) || 0,
      allowed_models: f.models.split(/[\s,，]+/).map((s) => s.trim()).filter(Boolean),
    }
    setBusy(true)
    try {
      const res = editing
        ? await api<{ key: ApiKey }>(`/admin/api/keys/${keyItem!.id}`, { method: 'PATCH', json: body })
        : await api<{ key: ApiKey }>('/admin/api/keys', { method: 'POST', json: body })
      toast.success(editing ? '已保存' : '密钥已创建')
      onOpenChange(false)
      onSaved(res.key)
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
            <DialogTitle className="text-[15px]">{editing ? '编辑密钥' : '创建密钥'}</DialogTitle>
            <DialogDescription className="text-[13px]">
              {editing ? (keyItem?.owner_email ? `属于用户 ${keyItem.owner_email}，扣费从其余额扣除。` : '管理员密钥，不扣任何用户余额。') : '管理员密钥不属于任何用户，只受下面的额度与限制约束。'}
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="名称" htmlFor="ak-name" className="sm:col-span-2">
              <Input id="ak-name" autoFocus value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} placeholder="例如 内部服务" />
            </Field>
            <Field label="额度上限（计费 Token）" htmlFor="ak-limit" hint="0 或留空表示不限">
              <Input id="ak-limit" inputMode="numeric" value={f.limit} onChange={(e) => setF({ ...f, limit: e.target.value.replace(/[^\d]/g, '') })} placeholder="不限" />
            </Field>
            <Field label="Key 倍率" htmlFor="ak-mult" hint="叠乘在模型倍率之上">
              <Input id="ak-mult" inputMode="decimal" value={f.multiplier} onChange={(e) => setF({ ...f, multiplier: e.target.value })} />
            </Field>
            <Field label="到期日" htmlFor="ak-exp" hint="当天 23:59 后失效；留空永不过期">
              <Input id="ak-exp" type="date" value={f.expires} onChange={(e) => setF({ ...f, expires: e.target.value })} className="[color-scheme:dark]" />
            </Field>
            <Field label="每分钟请求数（RPM）" htmlFor="ak-rpm" hint="0 或留空表示不单独限制">
              <Input id="ak-rpm" inputMode="numeric" value={f.rpm} onChange={(e) => setF({ ...f, rpm: e.target.value.replace(/[^\d]/g, '') })} placeholder="不限" />
            </Field>
            <Field label="允许的模型" htmlFor="ak-models" className="sm:col-span-2" hint="每行或逗号分隔一个模型 ID；留空允许全部模型。">
              <Textarea id="ak-models" rows={3} value={f.models} onChange={(e) => setF({ ...f, models: e.target.value })} className="font-mono text-xs" placeholder="gemini-3.5-flash-lite" />
            </Field>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button type="submit" disabled={busy}>
              {editing ? '保存' : '创建'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
