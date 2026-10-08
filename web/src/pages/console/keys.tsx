import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Copy, Eye, EyeOff, KeyRound, Pause, Pencil, Play, Plus, RefreshCw, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { PageHeader, Section, KV } from '@/components/page'
import { EmptyState, ErrorState, TableSkeleton } from '@/components/states'
import { RowMenu } from '@/components/row-menu'
import { StatusDot } from '@/components/status'
import { CopyButton, copyText } from '@/components/copy'
import { Field } from '@/components/fields'
import { QuickStart } from '@/components/quickstart'
import { useConfirm } from '@/components/confirm'
import { QuotaBar } from '@/components/quota-bar'
import { useNewParam } from '@/hooks/use-misc'
import { api } from '@/lib/api'
import { compact, dateTime, maskKey, num, relative } from '@/lib/format'
import type { ApiKey } from '@/lib/types'

function keyStatus(k: ApiKey) {
  if (!k.enabled) return <StatusDot tone="muted">已停用</StatusDot>
  if (k.quota_exhausted) return <StatusDot tone="danger">额度用尽</StatusDot>
  return <StatusDot tone="success">可用</StatusDot>
}

export default function ConsoleKeys() {
  const qc = useQueryClient()
  const confirm = useConfirm()
  const q = useQuery({ queryKey: ['user', 'keys'], queryFn: () => api<{ keys: ApiKey[]; max_keys: number }>('/api/user/keys') })
  const [createOpen, setCreateOpen] = useNewParam()
  const [edit, setEdit] = useState<ApiKey | null>(null)
  const [detail, setDetail] = useState<ApiKey | null>(null)
  const [created, setCreated] = useState<ApiKey | null>(null)
  const invalidate = () => qc.invalidateQueries({ queryKey: ['user'] })

  const patch = useMutation({
    mutationFn: ({ id, body }: { id: number; body: Record<string, unknown> }) => api<{ key: ApiKey }>(`/api/user/keys/${id}`, { method: 'PATCH', json: body }),
    onSuccess: () => invalidate(),
    onError: (e) => toast.error(e.message),
  })

  const actions = (k: ApiKey) => [
    { label: '复制密钥', icon: <Copy />, onSelect: () => copyText(k.key, '密钥已复制') },
    { label: '编辑名称与额度', icon: <Pencil />, onSelect: () => setEdit(k) },
    k.enabled
      ? { label: '停用', icon: <Pause />, onSelect: () => patch.mutate({ id: k.id, body: { enabled: false } }, { onSuccess: () => toast.success('已停用') }) }
      : { label: '启用', icon: <Play />, onSelect: () => patch.mutate({ id: k.id, body: { enabled: true } }, { onSuccess: () => toast.success('已启用') }) },
    {
      label: '重新生成',
      icon: <RefreshCw />,
      onSelect: async () => {
        if (!(await confirm({ title: '重新生成密钥？', description: '旧密钥会立即失效，正在使用它的程序需要换成新密钥。额度设置保持不变。', confirmText: '重新生成' }))) return
        try {
          const res = await api<{ key: ApiKey }>(`/api/user/keys/${k.id}/regenerate`, { method: 'POST' })
          setCreated(res.key)
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
        if (!(await confirm({ title: `删除密钥「${k.name || maskKey(k.key)}」？`, description: '删除后无法恢复，使用该密钥的请求会立即返回 401。历史用量记录会保留。', confirmText: '删除', destructive: true }))) return
        try {
          await api(`/api/user/keys/${k.id}`, { method: 'DELETE' })
          toast.success('密钥已删除')
          setDetail(null)
          invalidate()
        } catch (e) {
          toast.error((e as Error).message)
        }
      },
    },
  ]

  const keys = q.data?.keys || []
  return (
    <>
      <PageHeader
        title="API 密钥"
        description="用密钥调用 OpenAI 兼容接口，费用从你的账户余额扣除。可为单个密钥设置额度上限，避免某个程序用超。"
        actions={
          <Button onClick={() => setCreateOpen(true)} disabled={!!q.data && keys.length >= q.data.max_keys}>
            <Plus /> 创建密钥
          </Button>
        }
      />
      <Section
        title={q.data ? `${keys.length} 个密钥` : '密钥'}
        description={q.data ? `每个账号最多 ${q.data.max_keys} 个` : undefined}
      >
        {q.isLoading ? (
          <TableSkeleton rows={3} cols={5} />
        ) : q.isError ? (
          <ErrorState error={q.error} onRetry={() => q.refetch()} />
        ) : keys.length === 0 ? (
          <EmptyState
            icon={<KeyRound />}
            title="还没有 API 密钥"
            description="创建一个密钥，就可以在任意 OpenAI 兼容的客户端里使用。"
            action={
              <Button size="sm" onClick={() => setCreateOpen(true)}>
                <Plus /> 创建第一个密钥
              </Button>
            }
          />
        ) : (
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead>名称</TableHead>
                  <TableHead>状态</TableHead>
                  <TableHead>额度</TableHead>
                  <TableHead className="text-right">近 7 天</TableHead>
                  <TableHead>创建时间</TableHead>
                  <TableHead className="w-10" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {keys.map((k) => (
                  <TableRow key={k.id} className="cursor-pointer" onClick={() => setDetail(k)}>
                    <TableCell>
                      <div className="font-medium">{k.name || '未命名'}</div>
                      <div className="font-mono text-[11px] text-muted-foreground">{maskKey(k.key)}</div>
                    </TableCell>
                    <TableCell>{keyStatus(k)}</TableCell>
                    <TableCell>
                      <QuotaBar used={k.tokens_used} limit={k.token_limit} />
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      <div>{compact(k.charged_7d)}</div>
                      <div className="text-[11px] text-muted-foreground">{num(k.requests_7d)} 次</div>
                    </TableCell>
                    <TableCell className="text-muted-foreground">{relative(k.created_at)}</TableCell>
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

      <KeyFormDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onSaved={(k) => {
          setCreated(k)
          invalidate()
        }}
      />
      <KeyFormDialog open={!!edit} keyItem={edit} onOpenChange={(o) => !o && setEdit(null)} onSaved={() => invalidate()} />

      <Dialog open={!!created} onOpenChange={(o) => !o && setCreated(null)}>
        <DialogContent className="sm:max-w-[520px]">
          <DialogHeader>
            <DialogTitle className="text-[15px]">密钥已就绪</DialogTitle>
            <DialogDescription className="text-[13px]">复制下面的密钥配置到你的客户端。之后也可以在列表里随时复制。</DialogDescription>
          </DialogHeader>
          {created && (
            <div className="space-y-3">
              <div className="flex items-center gap-2 rounded-md border bg-[#0a0a0c] px-3 py-2">
                <code className="flex-1 truncate font-mono text-[12.5px] text-foreground">{created.key}</code>
                <CopyButton value={created.key} label="密钥已复制" />
              </div>
              <QuickStart apiKey={created.key} />
            </div>
          )}
          <DialogFooter>
            <Button onClick={() => setCreated(null)}>完成</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <KeyDetailSheet keyItem={detail} onOpenChange={(o) => !o && setDetail(null)} actions={detail ? actions(detail) : []} />
    </>
  )
}

function KeyFormDialog({ open, onOpenChange, keyItem, onSaved }: { open: boolean; onOpenChange: (o: boolean) => void; keyItem?: ApiKey | null; onSaved: (k: ApiKey) => void }) {
  const editing = !!keyItem
  const [name, setName] = useState('')
  const [limit, setLimit] = useState('')
  const [busy, setBusy] = useState(false)
  const [lastOpen, setLastOpen] = useState(false)
  if (open !== lastOpen) {
    setLastOpen(open)
    if (open) {
      setName(keyItem?.name || '')
      setLimit(keyItem?.token_limit ? String(keyItem.token_limit) : '')
    }
  }
  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    const tokenLimit = limit.trim() === '' ? 0 : Number(limit)
    if (!Number.isInteger(tokenLimit) || tokenLimit < 0) {
      toast.error('额度必须是非负整数')
      return
    }
    setBusy(true)
    try {
      const res = editing
        ? await api<{ key: ApiKey }>(`/api/user/keys/${keyItem!.id}`, { method: 'PATCH', json: { name, token_limit: tokenLimit } })
        : await api<{ key: ApiKey }>('/api/user/keys', { method: 'POST', json: { name, token_limit: tokenLimit } })
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
      <DialogContent className="sm:max-w-[420px]">
        <form onSubmit={submit} className="space-y-4">
          <DialogHeader>
            <DialogTitle className="text-[15px]">{editing ? '编辑密钥' : '创建 API 密钥'}</DialogTitle>
            <DialogDescription className="text-[13px]">名称只用于区分用途；额度是该密钥最多可扣的计费 Token。</DialogDescription>
          </DialogHeader>
          <Field label="名称" htmlFor="k-name">
            <Input id="k-name" autoFocus value={name} maxLength={60} onChange={(e) => setName(e.target.value)} placeholder="例如：笔记本 / 生产服务" />
          </Field>
          <Field label="额度上限（可选）" htmlFor="k-limit" hint="留空或 0 表示不单独限额，只受账户余额约束。">
            <Input id="k-limit" inputMode="numeric" value={limit} onChange={(e) => setLimit(e.target.value.replace(/[^\d]/g, ''))} placeholder="不限" />
          </Field>
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

function KeyDetailSheet({ keyItem, onOpenChange, actions }: { keyItem: ApiKey | null; onOpenChange: (o: boolean) => void; actions: { label: string; icon?: React.ReactNode; onSelect: () => void; destructive?: boolean }[] }) {
  const [reveal, setReveal] = useState(false)
  return (
    <Sheet open={!!keyItem} onOpenChange={(o) => { setReveal(false); onOpenChange(o) }}>
      <SheetContent className="w-full gap-0 sm:max-w-[480px]">
        {keyItem && (
          <>
            <SheetHeader className="border-b">
              <SheetTitle className="text-[15px]">{keyItem.name || '未命名密钥'}</SheetTitle>
              <SheetDescription className="text-xs">创建于 {dateTime(keyItem.created_at)}</SheetDescription>
            </SheetHeader>
            <div className="space-y-5 overflow-y-auto p-4">
              <div className="flex items-center gap-1 rounded-md border bg-[#0a0a0c] py-1 pr-1 pl-3">
                <code className="flex-1 truncate font-mono text-[12px]">{reveal ? keyItem.key : maskKey(keyItem.key)}</code>
                <Button variant="ghost" size="icon-sm" onClick={() => setReveal(!reveal)} aria-label={reveal ? '隐藏' : '显示'}>
                  {reveal ? <EyeOff /> : <Eye />}
                </Button>
                <CopyButton value={keyItem.key} label="密钥已复制" />
              </div>
              <KV
                items={[
                  ['状态', keyStatus(keyItem)],
                  ['额度', keyItem.token_limit ? `${num(keyItem.tokens_used)} / ${num(keyItem.token_limit)}` : `不限额（已用 ${num(keyItem.tokens_used)}）`],
                  ['近 7 天', `${num(keyItem.charged_7d)} Token · ${num(keyItem.requests_7d)} 次请求`],
                ]}
              />
              <div>
                <div className="mb-2 text-xs font-medium">调用示例</div>
                <QuickStart apiKey={reveal ? keyItem.key : undefined} />
              </div>
              <div className="flex flex-wrap gap-2 border-t pt-4">
                {actions.map((a) => (
                  <Button key={a.label} size="sm" variant={a.destructive ? 'destructive' : 'outline'} onClick={a.onSelect}>
                    {a.icon}
                    {a.label}
                  </Button>
                ))}
              </div>
            </div>
          </>
        )}
      </SheetContent>
    </Sheet>
  )
}
