import { useState } from 'react'
import { useSearchParams } from 'react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Ban, CheckCircle2, ChevronDown, KeyRound, Plus, RefreshCw, Server, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { PageHeader } from '@/components/page'
import { StatStrip } from '@/components/stat'
import { EmptyState, ErrorState, TableSkeleton } from '@/components/states'
import { type RowAction } from '@/components/row-menu'
import { accountHealth } from '@/components/account-status'
import { Field } from '@/components/fields'
import { ProviderIcon } from '@/components/provider-icon'
import { useConfirm } from '@/components/confirm'
import { api } from '@/lib/api'
import { ENGINE_PROVIDERS } from '@/lib/provider-presets'
import { canonicalEngine, engineLabel, type Account, type EngineName } from '@/lib/types'
import { AddAntigravityDialog, AddCodexDialog, AddCustomKeyDialog, AddKiroDialog, AddTeleAgentDialog, AddWorkBuddyDialog, CustomCredsDialog, SubCredsDialog } from './accounts-sub-engines'
import { ProviderModelsDialog } from './provider-models'
import { ProviderGroups } from './accounts-provider-groups'

export default function AdminAccounts() {
  const qc = useQueryClient()
  const confirm = useConfirm()
  const q = useQuery({ queryKey: ['admin', 'accounts'], queryFn: () => api<{ accounts: Account[] }>('/admin/api/accounts'), refetchInterval: 15_000 })
  // ?new=<engine> 来自命令面板或下拉菜单：直接打开对应的添加弹窗
  const [params, setParams] = useSearchParams()
  const newParam = canonicalEngine(params.get('new') || '')
  const ENGINE_KEYS: EngineName[] = ['geminiweb', 'aistudio', 'codex', 'antigravity', 'kiro', 'workbuddy', 'workbuddy-ai', 'teleagent', 'custom']
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
  const [checkingIn, setCheckingIn] = useState<number | null>(null)
  const [openProvider, setOpenProvider] = useState<{ engine: string; provider?: string } | null>(null)
  const [collapsed, setCollapsed] = useState<Set<string>>(() => {
    try {
      const saved: unknown = JSON.parse(localStorage.getItem('web2api.accounts.collapsed') || '[]')
      return new Set(Array.isArray(saved) ? saved.filter((v): v is string => typeof v === 'string') : [])
    } catch { return new Set() }
  })
  const invalidate = () => qc.invalidateQueries({ queryKey: ['admin'] })

  const check = async (a: Account) => {
    setChecking(a.id)
    try {
      await api(`/admin/api/accounts/${a.id}/check`, { method: 'POST' })
      toast.success(`${a.label} 已触发检查，请等待状态更新`)
    } catch (e) {
      toast.error(`${a.label}：${(e as Error).message}`)
    } finally {
      setChecking(null)
      invalidate()
    }
  }
  const checkIn = async (a: Account) => {
    setCheckingIn(a.id)
    try {
      const res = await api<{ result?: string }>(`/admin/api/accounts/${a.id}/checkin`, { method: 'POST' })
      toast.success(`${a.label}：${res.result || '签到成功'}`)
    } catch (e) {
      toast.error(`${a.label}：${(e as Error).message}`)
    } finally {
      setCheckingIn(null)
      invalidate()
    }
  }
  const toggleAutoCheckIn = async (a: Account, enabled: boolean) => {
    try {
      await api(`/admin/api/accounts/${a.id}/auto-checkin`, { method: 'PATCH', json: { enabled } })
      toast.success(`${a.label}：自动签到已${enabled ? '开启' : '关闭'}`)
      invalidate()
    } catch (e) {
      toast.error((e as Error).message)
    }
  }
  const actions = (a: Account): RowAction[] => [
    { label: '立即检查', icon: <RefreshCw />, onSelect: () => check(a), disabled: checking === a.id },
    ...(canonicalEngine(a.engine) === 'geminiweb'
      ? [{ label: '更新 Cookie', icon: <KeyRound />, onSelect: () => setCreds(a) }]
      : a.engine === 'custom'
        ? [{ label: '更新凭据', icon: <KeyRound />, onSelect: () => setCustomCreds(a) }]
        : ['codex', 'antigravity', 'kiro', 'workbuddy', 'workbuddy-ai', 'teleagent'].includes(a.engine)
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
    { key: 'geminiweb', title: 'Gemini Web 网页号', hint: '粘贴 __Secure-1PSID / 1PSIDTS Cookie' },
    { key: 'aistudio', title: 'AI Studio 号', hint: '粘贴 Playwright storage_state JSON' },
    { key: 'codex', title: 'GPT（ChatGPT 订阅）', hint: 'OAuth 授权或粘贴 refresh_token' },
    { key: 'antigravity', title: 'Antigravity（Google 订阅）', hint: 'Google OAuth 授权或粘贴 refresh_token' },
    { key: 'kiro', title: 'Kiro（AWS CodeWhisperer）', hint: '粘贴 Kiro IDE 的 refresh_token' },
    { key: 'workbuddy', title: 'WorkBuddy（国服）', hint: '扫码授权或粘贴 accessToken' },
    { key: 'workbuddy-ai', title: 'WorkBuddy AI（国际服）', hint: '扫码授权或粘贴 accessToken' },
    { key: 'teleagent', title: 'TeleAgent（天翼 AI）', hint: '浏览器登录授权或粘贴 token' },
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
          <DropdownMenuItem key={item.key} onSelect={() => setAdding(item.key)} className="items-start gap-2.5 py-2">
            <ProviderIcon icon={ENGINE_PROVIDERS[item.key]?.icon} name={item.title} size="md" className="mt-0.5" />
            <span className="flex min-w-0 flex-col gap-0.5">
              <span className="text-[13px]">{item.title}</span>
              <span className="text-[11px] text-muted-foreground">{item.hint}</span>
            </span>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )

  return (
    <>
      <PageHeader title="供应商" description="先看服务额度，再展开管理账号。WorkBuddy 国服与国际服分别展示；模型测试限定当前引擎或第三方供应商，生产请求按模型权益与可用状态调度。" actions={addMenu} />
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
        <div>
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
                  try { localStorage.setItem('web2api.accounts.collapsed', JSON.stringify([...next])) } catch { /* Storage may be disabled. */ }
                  return next
                })
              }
              onOpen={setOpenProvider}
              onCheck={check}
              actions={actions}
              checkin={{ checkingIn, onCheckIn: checkIn, onToggleAutoCheckIn: toggleAutoCheckIn }}
            />
          )}
        </div>
        {accounts.length > 0 && <p className="text-xs text-muted-foreground">点服务标题折叠账号，点账号名称查看重置时间与检查操作；额度未知不会算作零。</p>}
      </div>
      <AddGeminiDialog open={adding === 'geminiweb'} onOpenChange={(o) => !o && setAdding(null)} onDone={invalidate} />
      <AddStudioDialog open={adding === 'aistudio'} onOpenChange={(o) => !o && setAdding(null)} onDone={invalidate} />
      <AddCodexDialog open={adding === 'codex'} onOpenChange={(o: boolean) => !o && setAdding(null)} onDone={invalidate} />
      <AddAntigravityDialog open={adding === 'antigravity'} onOpenChange={(o: boolean) => !o && setAdding(null)} onDone={invalidate} />
      <AddKiroDialog open={adding === 'kiro'} onOpenChange={(o: boolean) => !o && setAdding(null)} onDone={invalidate} />
      <AddWorkBuddyDialog open={adding === 'workbuddy' || adding === 'workbuddy-ai'} edition={adding === 'workbuddy-ai' ? 'workbuddy-ai' : 'workbuddy'} onOpenChange={(o: boolean) => !o && setAdding(null)} onDone={invalidate} />
      <AddTeleAgentDialog open={adding === 'teleagent'} onOpenChange={(o: boolean) => !o && setAdding(null)} onDone={invalidate} />
      <AddCustomKeyDialog open={adding === 'custom'} onOpenChange={(o: boolean) => !o && setAdding(null)} onDone={invalidate} />
      <CredsDialog account={creds} onOpenChange={(o) => !o && setCreds(null)} onDone={invalidate} />
      <SubCredsDialog account={subCreds} onOpenChange={(o) => !o && setSubCreds(null)} onDone={invalidate} />
      <CustomCredsDialog account={customCreds} onOpenChange={(o) => !o && setCustomCreds(null)} onDone={invalidate} />
      <ProviderModelsDialog
        target={openProvider}
        onOpenChange={(o) => !o && setOpenProvider(null)}
        onAddAccount={(engine) => setAdding(canonicalEngine(engine) as EngineName)}
      />
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
      await api('/admin/api/accounts', { method: 'POST', json: { engine: 'geminiweb', label: f.label.trim(), storage_state } })
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
      await api('/admin/api/accounts', { method: 'POST', json: { engine: 'aistudio', ...f } })
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
