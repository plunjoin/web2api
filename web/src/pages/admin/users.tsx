import { useState } from 'react'
import { Link } from 'react-router'
import { keepPreviousData, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Ban, CheckCircle2, KeyRound, Pencil, Plus, ReceiptText, Search, ShieldCheck, Trash2, Users, Wallet } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { PageHeader, Section, Toolbar, KV } from '@/components/page'
import { EmptyState, ErrorState, TableSkeleton, BlockSkeleton } from '@/components/states'
import { RowMenu, type RowAction } from '@/components/row-menu'
import { Pill, StatusDot } from '@/components/status'
import { Pager } from '@/components/pager'
import { MiniSelect } from '@/components/fields'
import { LedgerTable } from '@/components/ledger-table'
import { useConfirm } from '@/components/confirm'
import { useDebounced, useNewParam } from '@/hooks/use-misc'
import { api, qs } from '@/lib/api'
import { compact, dateTime, maskKey, multiplier, num, relative } from '@/lib/format'
import type { ApiKey, LedgerEntry, User, UsageTotals } from '@/lib/types'
import { cn } from '@/lib/utils'
import { AdjustBalanceDialog, CreateUserDialog, EditUserDialog, ResetPasswordDialog } from './user-dialogs'

const LIMIT = 50

export default function AdminUsers() {
  const qc = useQueryClient()
  const confirm = useConfirm()
  const [search, setSearch] = useState('')
  const q = useDebounced(search)
  const [role, setRole] = useState('all')
  const [status, setStatus] = useState('all')
  const [offset, setOffset] = useState(0)
  const [createOpen, setCreateOpen] = useNewParam()
  const [detailId, setDetailId] = useState<number | null>(null)
  const [edit, setEdit] = useState<User | null>(null)
  const [adjust, setAdjust] = useState<User | null>(null)
  const [resetPw, setResetPw] = useState<User | null>(null)

  const list = useQuery({
    queryKey: ['admin', 'users', q, role, status, offset],
    queryFn: () => api<{ users: User[]; total: number }>(`/admin/api/users${qs({ q, role, status, limit: LIMIT, offset })}`),
    placeholderData: keepPreviousData,
  })
  const invalidate = () => qc.invalidateQueries({ queryKey: ['admin'] })

  const setEnabled = async (u: User, enabled: boolean) => {
    if (!enabled && !(await confirm({ title: `停用 ${u.email}？`, description: '停用后该用户无法登录，其名下所有密钥的请求会返回 403。余额与记录保留，可随时重新启用。', confirmText: '停用', destructive: true }))) return
    try {
      await api(`/admin/api/users/${u.id}`, { method: 'PATCH', json: { enabled } })
      toast.success(enabled ? '已启用' : '已停用')
      invalidate()
    } catch (e) {
      toast.error((e as Error).message)
    }
  }
  const remove = async (u: User) => {
    if (!(await confirm({ title: `删除用户 ${u.email}？`, description: '会同时删除该用户的密钥和余额流水，且无法恢复。历史用量记录保留（显示为已删除用户）。如只想禁止使用，请选择“停用”。', confirmText: '永久删除', destructive: true }))) return
    try {
      await api(`/admin/api/users/${u.id}`, { method: 'DELETE' })
      toast.success('用户已删除')
      setDetailId(null)
      invalidate()
    } catch (e) {
      toast.error((e as Error).message)
    }
  }
  const actions = (u: User): RowAction[] => [
    { label: '调整余额', icon: <Wallet />, onSelect: () => setAdjust(u) },
    { label: '编辑资料与倍率', icon: <Pencil />, onSelect: () => setEdit(u) },
    { label: '重置密码', icon: <KeyRound />, onSelect: () => setResetPw(u) },
    u.enabled ? { label: '停用', icon: <Ban />, onSelect: () => setEnabled(u, false) } : { label: '启用', icon: <CheckCircle2 />, onSelect: () => setEnabled(u, true) },
    { label: '删除', icon: <Trash2 />, destructive: true, separatorBefore: true, onSelect: () => remove(u) },
  ]

  const users = list.data?.users || []
  const filtered = !!q || role !== 'all' || status !== 'all'
  return (
    <>
      <PageHeader
        title="用户"
        description="注册用户与他们的余额。点击一行查看密钥和流水。"
        actions={
          <Button onClick={() => setCreateOpen(true)}>
            <Plus /> 新建用户
          </Button>
        }
      />
      <Toolbar className="mb-3">
        <div className="relative">
          <Search className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input value={search} onChange={(e) => { setSearch(e.target.value); setOffset(0) }} placeholder="搜索邮箱、昵称或备注" className="h-7 w-64 pl-8 text-xs" />
        </div>
        <MiniSelect value={role} onChange={(v) => { setRole(v); setOffset(0) }} options={[{ value: 'all', label: '全部角色' }, { value: 'user', label: '普通用户' }, { value: 'admin', label: '管理员' }]} />
        <MiniSelect value={status} onChange={(v) => { setStatus(v); setOffset(0) }} options={[{ value: 'all', label: '全部状态' }, { value: 'enabled', label: '正常' }, { value: 'disabled', label: '已停用' }]} />
      </Toolbar>
      <Section title={list.data ? `${num(list.data.total)} 个用户` : '用户'}>
        {list.isLoading ? (
          <TableSkeleton rows={6} cols={7} />
        ) : list.isError ? (
          <ErrorState error={list.error} onRetry={() => list.refetch()} />
        ) : users.length === 0 ? (
          filtered ? (
            <EmptyState icon={<Search />} title="没有匹配的用户" description="换个关键词或清除筛选条件。" />
          ) : (
            <EmptyState
              icon={<Users />}
              title="还没有用户"
              description="可以在设置里开放注册，或直接在这里创建账号。"
              action={
                <div className="flex gap-2">
                  <Button size="sm" variant="outline" asChild>
                    <Link to="/admin/settings">注册设置</Link>
                  </Button>
                  <Button size="sm" onClick={() => setCreateOpen(true)}>
                    <Plus /> 新建用户
                  </Button>
                </div>
              }
            />
          )
        ) : (
          <>
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead>用户</TableHead>
                    <TableHead>状态</TableHead>
                    <TableHead className="text-right">余额</TableHead>
                    <TableHead className="text-right">倍率</TableHead>
                    <TableHead className="text-right">密钥</TableHead>
                    <TableHead className="text-right">7 天消耗</TableHead>
                    <TableHead>最近登录</TableHead>
                    <TableHead className="w-10" />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {users.map((u) => (
                    <TableRow key={u.id} className="cursor-pointer" onClick={() => setDetailId(u.id)}>
                      <TableCell>
                        <div className="flex items-center gap-1.5">
                          <span className="font-medium">{u.email}</span>
                          {u.role === 'admin' && <Pill tone="primary">管理员</Pill>}
                        </div>
                        <div className="text-[11px] text-muted-foreground">{u.nickname || '—'}{u.note ? ` · ${u.note}` : ''}</div>
                      </TableCell>
                      <TableCell>{u.enabled ? <StatusDot tone="success">正常</StatusDot> : <StatusDot tone="muted">已停用</StatusDot>}</TableCell>
                      <TableCell className={cn('text-right font-medium tabular-nums', u.balance <= 0 && 'text-destructive')}>{num(u.balance)}</TableCell>
                      <TableCell className="text-right text-muted-foreground tabular-nums">{multiplier(u.multiplier)}</TableCell>
                      <TableCell className="text-right tabular-nums">{u.key_count ?? 0}</TableCell>
                      <TableCell className="text-right tabular-nums">{compact(u.charged_7d)}</TableCell>
                      <TableCell className="text-muted-foreground">{u.last_login_at ? relative(u.last_login_at) : '从未'}</TableCell>
                      <TableCell onClick={(e) => e.stopPropagation()}>
                        <RowMenu actions={actions(u)} />
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

      <UserDetailSheet id={detailId} onOpenChange={(o) => !o && setDetailId(null)} actions={actions} />
      <CreateUserDialog open={createOpen} onOpenChange={setCreateOpen} onCreated={() => invalidate()} />
      <EditUserDialog user={edit} onOpenChange={(o) => !o && setEdit(null)} onSaved={invalidate} />
      <AdjustBalanceDialog user={adjust} onOpenChange={(o) => !o && setAdjust(null)} onSaved={invalidate} />
      <ResetPasswordDialog user={resetPw} onOpenChange={(o) => !o && setResetPw(null)} />
    </>
  )
}

function UserDetailSheet({ id, onOpenChange, actions }: { id: number | null; onOpenChange: (o: boolean) => void; actions: (u: User) => RowAction[] }) {
  const q = useQuery({
    queryKey: ['admin', 'user', id],
    queryFn: () => api<{ user: User; keys: ApiKey[]; ledger: LedgerEntry[]; week: UsageTotals }>(`/admin/api/users/${id}`),
    enabled: id !== null,
  })
  const d = q.data
  return (
    <Sheet open={id !== null} onOpenChange={onOpenChange}>
      <SheetContent className="w-full gap-0 sm:max-w-[640px]">
        <SheetHeader className="border-b">
          <SheetTitle className="flex items-center gap-2 text-[15px]">
            {d?.user.email || '用户详情'}
            {d?.user.role === 'admin' && <Pill tone="primary"><ShieldCheck className="size-3" />管理员</Pill>}
            {d && !d.user.enabled && <Pill>已停用</Pill>}
          </SheetTitle>
          <SheetDescription className="text-xs">{d ? `#${d.user.id} · 注册于 ${dateTime(d.user.created_at)}` : '加载中…'}</SheetDescription>
        </SheetHeader>
        <div className="space-y-5 overflow-y-auto p-4">
          {q.isLoading || !d ? (
            q.isError ? <ErrorState error={q.error} onRetry={() => q.refetch()} /> : <BlockSkeleton className="h-64" />
          ) : (
            <>
              <div className="grid grid-cols-3 divide-x rounded-lg border">
                <Metric label="余额" value={num(d.user.balance)} danger={d.user.balance <= 0} />
                <Metric label="7 天消耗" value={compact(d.week.charged_tokens)} hint={`${num(d.week.requests)} 次请求`} />
                <Metric label="累计充值" value={compact(d.user.total_recharge)} />
              </div>
              <div className="flex flex-wrap gap-2">
                {actions(d.user).map((a) => (
                  <Button key={a.label} size="sm" variant={a.destructive ? 'destructive' : 'outline'} onClick={a.onSelect}>
                    {a.icon}
                    {a.label}
                  </Button>
                ))}
              </div>
              <KV
                items={[
                  ['昵称', d.user.nickname || '—'],
                  ['用户倍率', multiplier(d.user.multiplier)],
                  ['最近登录', d.user.last_login_at ? dateTime(d.user.last_login_at) : '从未'],
                  ['备注', d.user.note || '—'],
                ]}
              />
              <div>
                <div className="mb-2 flex items-center justify-between">
                  <span className="text-xs font-medium">密钥（{d.keys.length}）</span>
                </div>
                {d.keys.length === 0 ? (
                  <p className="rounded-md border border-dashed px-3 py-4 text-center text-xs text-muted-foreground">该用户还没有创建密钥</p>
                ) : (
                  <div className="divide-y rounded-md border">
                    {d.keys.map((k) => (
                      <div key={k.id} className="flex items-center justify-between gap-3 px-3 py-2 text-[13px]">
                        <div className="min-w-0">
                          <div className="truncate">{k.name || '未命名'}</div>
                          <div className="font-mono text-[11px] text-muted-foreground">{maskKey(k.key)}</div>
                        </div>
                        <div className="text-right text-xs text-muted-foreground tabular-nums">
                          {k.enabled ? '启用' : '停用'} · 已用 {compact(k.tokens_used)}
                          {k.token_limit ? ` / ${compact(k.token_limit)}` : ''}
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
              <div>
                <div className="mb-2 flex items-center justify-between">
                  <span className="text-xs font-medium">最近流水</span>
                  <Button variant="ghost" size="sm" asChild>
                    <Link to={`/admin/ledger?user_id=${d.user.id}`}>
                      <ReceiptText /> 全部流水
                    </Link>
                  </Button>
                </div>
                {d.ledger.length === 0 ? (
                  <p className="rounded-md border border-dashed px-3 py-4 text-center text-xs text-muted-foreground">暂无流水</p>
                ) : (
                  <div className="rounded-md border">
                    <LedgerTable entries={d.ledger} />
                  </div>
                )}
              </div>
            </>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}

function Metric({ label, value, hint, danger }: { label: string; value: string; hint?: string; danger?: boolean }) {
  return (
    <div className="px-3 py-2.5">
      <div className="text-[11px] text-muted-foreground">{label}</div>
      <div className={cn('mt-0.5 text-lg font-semibold tabular-nums', danger && 'text-destructive')}>{value}</div>
      {hint && <div className="text-[11px] text-muted-foreground">{hint}</div>}
    </div>
  )
}
