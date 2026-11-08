import { useState } from 'react'
import { useSearchParams } from 'react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Ban, CheckCircle2, ChevronDown, ChevronRight, KeyRound, Plus, RefreshCw, Server, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Table, TableBody, TableCell, TableRow } from '@/components/ui/table'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { PageHeader, Section } from '@/components/page'
import { StatStrip } from '@/components/stat'
import { EmptyState, ErrorState, TableSkeleton } from '@/components/states'
import { RowMenu, type RowAction } from '@/components/row-menu'
import { StatusDot } from '@/components/status'
import { AccountStatus, CooldownLines, TierPill, accountHealth } from '@/components/account-status'
import { Field } from '@/components/fields'
import { useConfirm } from '@/components/confirm'
import { api } from '@/lib/api'
import { relative } from '@/lib/format'
import { engineLabel, type Account, type EngineName } from '@/lib/types'
import { AddAntigravityDialog, AddCodexDialog, AddCustomKeyDialog, AddKiroDialog, AddWorkBuddyDialog, CustomCredsDialog, SubCredsDialog } from './accounts-sub-engines'
import { ProviderModelsDialog } from './provider-models'

// 供应商分组顺序：一个供应商下可以挂多个账号，请求在就绪账号间轮询。
const PROVIDER_ORDER: EngineName[] = ['a', 'b', 'codex', 'antigravity', 'kiro', 'workbuddy', 'custom']

export default function AdminAccounts() {
  const qc = useQueryClient()
  const confirm = useConfirm()
  const q = useQuery({ queryKey: ['admin', 'accounts'], queryFn: () => api<{ accounts: Account[] }>('/admin/api/accounts'), refetchInterval: 15_000 })
  // ?new=<engine> 来自命令面板或下拉菜单：直接打开对应的添加弹窗
  const [params, setParams] = useSearchParams()
  const newParam = params.get('new')
  const ENGINE_KEYS: EngineName[] = ['a', 'b', 'codex', 'antigravity', 'kiro', 'workbuddy', 'custom']
  const adding = ENGINE_KEYS.includes(newParam as EngineName) ? (newParam as EngineName) : null
  const setAdding = (v: EngineName | null) => {
    const next = new URLSearchParams(params)
    if (v) next.set('new', v)
    else next.delete('new')
    setParams(next, { replace: true })
  }
  const [creds, setCreds] = useState<Account | null>(null)
  const [subCreds, setSubCreds] = useState<Account | null>(null)
  const [customCreds, setCustomCreds] = useState<Account | null>(null)
  const [checking, setChecking] = useState<number | null>(null)
  const [openProvider, setOpenProvider] = useState<{ engine: string; provider?: string } | null>(null)
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set())
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
    ...(a.engine === 'a'
      ? [{ label: '更新 Cookie', icon: <KeyRound />, onSelect: () => setCreds(a) }]
      : a.engine === 'custom'
        ? [{ label: '更新凭据', icon: <KeyRound />, onSelect: () => setCustomCreds(a) }]
        : ['codex', 'antigravity', 'kiro', 'workbuddy'].includes(a.engine)
          ? [{ label: '更新凭据', icon: <KeyRound />, onSelect: () => setSubCreds(a) }]
          : []),
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
  const ready = accounts.filter((a) => accountHealth(a) === 'up').length
  const broken = accounts.filter((a) => accountHealth(a) === 'down').length
  const engineCounts = accounts.reduce<Record<string, number>>((m, a) => {
    m[a.engine] = (m[a.engine] || 0) + 1
    return m
  }, {})
  const engineHint = Object.entries(engineCounts)
    .map(([k, n]) => `${engineLabel(k)} ${n}`)
    .join(' · ')
  const addMenuItems: { key: EngineName; title: string; hint: string }[] = [
    { key: 'a', title: 'Gemini Web 网页号', hint: '粘贴 __Secure-1PSID / 1PSIDTS Cookie' },
    { key: 'b', title: 'AI Studio 号', hint: '粘贴 Playwright storage_state JSON' },
    { key: 'codex', title: 'GPT（ChatGPT 订阅）', hint: 'OAuth 授权或粘贴 refresh_token' },
    { key: 'antigravity', title: 'Antigravity（Google 订阅）', hint: 'Google OAuth 授权或粘贴 refresh_token' },
    { key: 'kiro', title: 'Kiro（AWS CodeWhisperer）', hint: '粘贴 Kiro IDE 的 refresh_token' },
    { key: 'workbuddy', title: 'WorkBuddy / CodeBuddy（腾讯）', hint: '扫码授权或粘贴 accessToken' },
    { key: 'custom', title: '第三方 Key（OpenAI 兼容）', hint: '填 Base URL + API Key' },
  ]
  const addMenu = (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button>
          <Plus /> 添加账号 <ChevronDown className="opacity-60" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-72">
        {addMenuItems.map((item) => (
          <DropdownMenuItem key={item.key} onSelect={() => setAdding(item.key)} className="flex-col items-start gap-0.5">
            <span className="text-[13px]">{item.title}</span>
            <span className="text-[11px] text-muted-foreground">{item.hint}</span>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )

  return (
    <>
      <PageHeader title="供应商" description="按供应商分组，一个供应商下可以添加多个账号，请求在就绪账号之间轮询。点开供应商即可获取它的模型、勾选要开放的、并直接测试哪些可用。" actions={addMenu} />
      <div className="space-y-4">
        <StatStrip
          className="lg:grid-cols-3"
          loading={q.isLoading}
          items={[
            { label: '账号总数', value: accounts.length, hint: engineHint || '还没有账号' },
            { label: '可用', value: ready, tone: 'success', hint: '就绪或正在处理请求' },
            { label: '异常', value: broken, tone: broken ? 'danger' : 'default', hint: broken ? '检查凭据是否过期' : '没有异常账号' },
          ]}
        />
        <Section>
          {q.isLoading ? (
            <TableSkeleton rows={4} cols={6} />
          ) : q.isError ? (
            <ErrorState error={q.error} onRetry={() => q.refetch()} />
          ) : accounts.length === 0 ? (
            <EmptyState icon={<Server />} title="还没有供应商账号" description="至少添加一个账号，网关才能处理请求。" action={addMenu} />
          ) : (
            <ProviderGroups
              accounts={accounts}
              checking={checking}
              collapsed={collapsed}
              onToggle={(key) =>
                setCollapsed((prev) => {
                  const next = new Set(prev)
                  if (next.has(key)) next.delete(key)
                  else next.add(key)
                  return next
                })
              }
              onOpen={setOpenProvider}
              onCheck={check}
              actions={actions}
            />
          )}
        </Section>
        {accounts.length > 0 && <p className="text-xs text-muted-foreground">点开供应商管理它的模型；点某个账号立即检查该账号。</p>}
      </div>
      <AddGeminiDialog open={adding === 'a'} onOpenChange={(o) => !o && setAdding(null)} onDone={invalidate} />
      <AddStudioDialog open={adding === 'b'} onOpenChange={(o) => !o && setAdding(null)} onDone={invalidate} />
      <AddCodexDialog open={adding === 'codex'} onOpenChange={(o: boolean) => !o && setAdding(null)} onDone={invalidate} />
      <AddAntigravityDialog open={adding === 'antigravity'} onOpenChange={(o: boolean) => !o && setAdding(null)} onDone={invalidate} />
      <AddKiroDialog open={adding === 'kiro'} onOpenChange={(o: boolean) => !o && setAdding(null)} onDone={invalidate} />
      <AddWorkBuddyDialog open={adding === 'workbuddy'} onOpenChange={(o: boolean) => !o && setAdding(null)} onDone={invalidate} />
      <AddCustomKeyDialog open={adding === 'custom'} onOpenChange={(o: boolean) => !o && setAdding(null)} onDone={invalidate} />
      <CredsDialog account={creds} onOpenChange={(o) => !o && setCreds(null)} onDone={invalidate} />
      <SubCredsDialog account={subCreds} onOpenChange={(o) => !o && setSubCreds(null)} onDone={invalidate} />
      <CustomCredsDialog account={customCreds} onOpenChange={(o) => !o && setCustomCreds(null)} onDone={invalidate} />
      <ProviderModelsDialog
        target={openProvider}
        onOpenChange={(o) => !o && setOpenProvider(null)}
        onAddAccount={(engine) => setAdding(engine as EngineName)}
      />
    </>
  )
}

// 第三方 Key 不是一个供应商：每个上游地址是一个供应商，其下挂多个 Key。
// 分组键用归一化后的 base_url；地址缺失时单独成组，避免和真供应商混在一起。
const UNKNOWN_PROVIDER = 'custom|unknown'

function providerOf(a: Account): string {
  return a.engine === 'custom' ? a.provider || UNKNOWN_PROVIDER : a.engine
}

function providerLabel(key: string, engine: string): string {
  if (engine !== 'custom') return engineLabel(engine)
  return key === UNKNOWN_PROVIDER ? '第三方 Key（未识别地址）' : key.replace(/^custom\|/, '')
}

function ProviderGroups({
  accounts,
  checking,
  collapsed,
  onToggle,
  onOpen,
  onCheck,
  actions,
}: {
  accounts: Account[]
  checking: number | null
  collapsed: Set<string>
  onToggle: (key: string) => void
  onOpen: (target: { engine: string; provider?: string }) => void
  onCheck: (a: Account) => void
  actions: (a: Account) => RowAction[]
}) {
  const groups = new Map<string, Account[]>()
  for (const a of accounts) {
    const key = providerOf(a)
    const list = groups.get(key) || []
    list.push(a)
    groups.set(key, list)
  }
  const keys = [...groups.keys()].sort((x, y) => {
    const xe = groups.get(x)?.[0]?.engine || ''
    const ye = groups.get(y)?.[0]?.engine || ''
    const byEngine = (PROVIDER_ORDER.indexOf(xe as EngineName) + 1 || 99) - (PROVIDER_ORDER.indexOf(ye as EngineName) + 1 || 99)
    return byEngine || providerLabel(x, xe).localeCompare(providerLabel(y, ye))
  })
  return (
    <div className="divide-y">
      {keys.map((key) => {
        const list = groups.get(key) || []
        const engine = list[0]?.engine || key
        const up = list.filter((a) => accountHealth(a) === 'up').length
        const isCollapsed = collapsed.has(key)
        return (
          <div key={key}>
            <div className="flex items-center gap-2 bg-muted/30 px-4 py-2">
              <button type="button" className="flex min-w-0 items-center gap-1.5 rounded px-1 py-0.5 hover:bg-accent" onClick={() => onToggle(key)} aria-expanded={!isCollapsed}>
                {isCollapsed ? <ChevronRight className="size-3.5 shrink-0 text-muted-foreground" /> : <ChevronDown className="size-3.5 shrink-0 text-muted-foreground" />}
                <span className="truncate text-[13px] font-medium" title={providerLabel(key, engine)}>
                  {providerLabel(key, engine)}
                </span>
              </button>
              {engine === 'custom' && key !== UNKNOWN_PROVIDER && (
                <span className="shrink-0 text-[11px] text-muted-foreground">第三方 Key</span>
              )}
              <span className="shrink-0 text-[11px] text-muted-foreground tabular-nums">
                {list.length} 个账号 · 可用 {up}
              </span>
              <Button
                size="sm"
                variant="outline"
                className="ml-auto h-6 shrink-0 px-2 text-xs"
                disabled={key === UNKNOWN_PROVIDER}
                title={key === UNKNOWN_PROVIDER ? '这些账号没有可识别的上游地址，先更新凭据再获取模型' : undefined}
                onClick={() => onOpen({ engine, provider: engine === 'custom' ? key : undefined })}
              >
                模型
              </Button>
            </div>
            {!isCollapsed && (
              <Table>
                <TableBody>
                  {list.map((a) => {
                    const detail = a.live?.detail || a.detail
                    return (
                      <TableRow key={a.id} className="cursor-pointer" onClick={() => onCheck(a)}>
                        <TableCell>
                          <div className="font-medium">{a.label}</div>
                          <div className="text-[11px] text-muted-foreground">#{a.id}</div>
                        </TableCell>
                        <TableCell className="max-w-[320px]">
                          {checking === a.id ? (
                            <StatusDot tone="warning">检查中…</StatusDot>
                          ) : detail && accountHealth(a) !== 'up' ? (
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <span>
                                  <AccountStatus account={a} />
                                </span>
                              </TooltipTrigger>
                              <TooltipContent className="max-w-sm">{detail}</TooltipContent>
                            </Tooltip>
                          ) : (
                            <AccountStatus account={a} />
                          )}
                          {detail && accountHealth(a) === 'down' && <div className="truncate text-[11px] text-muted-foreground">{detail}</div>}
                          <CooldownLines cooldowns={a.live?.cooldowns} />
                        </TableCell>
                        <TableCell className="max-w-[240px] whitespace-normal">
                          <div className="flex min-w-0 flex-col items-start gap-1">
                            <TierPill tier={a.live?.tier} />
                            {a.live?.quota && (
                              <span className="line-clamp-2 break-all text-[11px] text-muted-foreground tabular-nums" title={a.live.quota}>
                                {a.live.quota}
                              </span>
                            )}
                          </div>
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
            )}
          </div>
        )
      })}
    </div>
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
            <DialogTitle className="text-[15px]">添加 Gemini Web 网页号</DialogTitle>
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
