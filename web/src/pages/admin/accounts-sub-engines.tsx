import { useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'
import { ExternalLink, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Field } from '@/components/fields'
import { api } from '@/lib/api'

type OAuthStartResp = { engine: string; session_id: string; auth_url: string; expires_in: number }

/** OAuth 授权链接展示块（codex / antigravity / workbuddy 共用）。 */
function OAuthLinkBlock({ engine, url, onExpired }: { engine: string; url: string; onExpired?: () => void }) {
  return (
    <div className="space-y-2 rounded-md border bg-muted/30 p-3">
      <div className="flex items-center justify-between gap-2">
        <span className="text-[12px] font-medium">第 1 步：打开授权页面并登录</span>
        <Button type="button" variant="outline" size="sm" onClick={() => window.open(url, '_blank')}>
          <ExternalLink /> 打开授权页面
        </Button>
      </div>
      <Input readOnly value={url} className="font-mono text-[11px]" onFocus={(e) => e.currentTarget.select()} />
      {engine !== 'workbuddy' && (
        <p className="text-[11px] leading-relaxed text-muted-foreground">
          登录并授权后，浏览器会跳转到一个 localhost 地址（页面可能打不开，属正常现象）。把地址栏的完整 URL 复制粘贴到下方输入框即可。
        </p>
      )}
      {engine === 'workbuddy' && (
        <p className="text-[11px] leading-relaxed text-muted-foreground">在打开的页面中扫码或登录 CodeBuddy 账号，本弹窗会自动检测授权结果。</p>
      )}
      {onExpired && (
        <button type="button" className="text-[11px] text-muted-foreground underline-offset-2 hover:underline" onClick={onExpired}>
          链接失效了？重新生成
        </button>
      )}
    </div>
  )
}

/** OAuth 授权链接式添加（codex / antigravity）。 */
export function AddOAuthEngineDialog({
  engine,
  title,
  description,
  hint,
  open,
  onOpenChange,
  onDone,
}: {
  engine: 'codex' | 'antigravity'
  title: string
  description: string
  hint: string
  open: boolean
  onOpenChange: (o: boolean) => void
  onDone: () => void
}) {
  const [mode, setMode] = useState<'oauth' | 'paste'>('oauth')
  const [session, setSession] = useState<OAuthStartResp | null>(null)
  const [code, setCode] = useState('')
  const [pasteToken, setPasteToken] = useState('')
  const [pasteEmail, setPasteEmail] = useState('')
  const [label, setLabel] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!open) {
      setSession(null)
      setCode('')
      setPasteToken('')
      setPasteEmail('')
      setLabel('')
      setMode('oauth')
    }
  }, [open])

  const startAuthorize = async () => {
    setBusy(true)
    try {
      const resp = await api<OAuthStartResp>(`/admin/api/oauth/${engine}/authorize`, { method: 'POST', json: {} })
      setSession(resp)
      window.open(resp.auth_url, '_blank')
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const exchange = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!session || !code.trim()) return
    setBusy(true)
    try {
      await api(`/admin/api/oauth/${engine}/exchange`, { method: 'POST', json: { session_id: session.session_id, code: code.trim(), label: label.trim(), email: pasteEmail.trim() } })
      toast.success('账号已添加，正在初始化')
      onOpenChange(false)
      onDone()
    } catch (err) {
      toast.error((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const pasteSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!pasteToken.trim()) return
    setBusy(true)
    try {
      await api('/admin/api/accounts', {
        method: 'POST',
        json: { engine, label: label.trim(), email: pasteEmail.trim(), credentials: { refresh_token: pasteToken.trim(), email: pasteEmail.trim() } },
      })
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
        <div className="mb-2 flex gap-1 rounded-md bg-muted p-1 text-[12px]">
          <button type="button" className={`flex-1 rounded px-2 py-1 ${mode === 'oauth' ? 'bg-background shadow-sm font-medium' : 'text-muted-foreground'}`} onClick={() => setMode('oauth')}>
            OAuth 授权（推荐）
          </button>
          <button type="button" className={`flex-1 rounded px-2 py-1 ${mode === 'paste' ? 'bg-background shadow-sm font-medium' : 'text-muted-foreground'}`} onClick={() => setMode('paste')}>
            粘贴 refresh_token
          </button>
        </div>
        {mode === 'oauth' ? (
          <form onSubmit={exchange} className="space-y-4">
            <DialogHeader>
              <DialogTitle className="text-[15px]">{title}</DialogTitle>
              <DialogDescription className="text-[13px]">{description}</DialogDescription>
            </DialogHeader>
            {!session ? (
              <Button type="button" onClick={startAuthorize} disabled={busy} className="w-full">
                {busy ? <Loader2 className="animate-spin" /> : null} 生成授权链接
              </Button>
            ) : (
              <>
                <OAuthLinkBlock engine={engine} url={session.auth_url} onExpired={startAuthorize} />
                <Field label="第 2 步：粘贴回调 URL 或授权码" htmlFor={`oc-code-${engine}`}>
                  <Textarea
                    id={`oc-code-${engine}`}
                    rows={2}
                    required
                    autoFocus
                    spellCheck={false}
                    value={code}
                    onChange={(e) => setCode(e.target.value)}
                    className="font-mono text-[11px]"
                    placeholder="http://localhost:1455/auth/callback?code=...&state=... 或直接粘贴 code"
                  />
                </Field>
                <Field label="名称（可选）" htmlFor={`oc-label-${engine}`}>
                  <Input id={`oc-label-${engine}`} value={label} onChange={(e) => setLabel(e.target.value)} placeholder="留空使用账号邮箱" />
                </Field>
              </>
            )}
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                取消
              </Button>
              <Button type="submit" disabled={busy || !session || !code.trim()}>
                完成授权
              </Button>
            </DialogFooter>
          </form>
        ) : (
          <form onSubmit={pasteSubmit} className="space-y-4">
            <DialogHeader>
              <DialogTitle className="text-[15px]">{title}</DialogTitle>
              <DialogDescription className="text-[13px]">{hint}</DialogDescription>
            </DialogHeader>
            <Field label="refresh_token" htmlFor={`pt-${engine}`}>
              <Textarea id={`pt-${engine}`} rows={3} required autoFocus spellCheck={false} value={pasteToken} onChange={(e) => setPasteToken(e.target.value)} className="font-mono text-[11px]" />
            </Field>
            <div className="grid gap-3 sm:grid-cols-2">
              <Field label="名称（可选）" htmlFor={`pt-label-${engine}`}>
                <Input id={`pt-label-${engine}`} value={label} onChange={(e) => setLabel(e.target.value)} placeholder="留空使用邮箱" />
              </Field>
              <Field label="邮箱（可选）" htmlFor={`pt-email-${engine}`}>
                <Input id={`pt-email-${engine}`} value={pasteEmail} onChange={(e) => setPasteEmail(e.target.value)} placeholder="user@example.com" />
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
        )}
      </DialogContent>
    </Dialog>
  )
}

/** Codex 账号添加弹窗（OAuth 授权 + 粘贴 refresh_token 双模式）。 */
export function AddCodexDialog(props: Omit<Parameters<typeof AddOAuthEngineDialog>[0], 'engine' | 'title' | 'description' | 'hint'>) {
  return (
    <AddOAuthEngineDialog
      {...props}
      engine="codex"
      title="添加 Codex 账号"
      description="登录 ChatGPT 账号并授权后，把浏览器跳转的 localhost 地址完整粘贴回来；也可以直接粘贴 Codex CLI 的 refresh_token。"
      hint="从 Codex CLI 的 ~/.codex/auth.json 中复制 refresh_token 字段。"
    />
  )
}

/** Antigravity 账号添加弹窗（Google OAuth 授权 + 粘贴 refresh_token 双模式）。 */
export function AddAntigravityDialog(props: Omit<Parameters<typeof AddOAuthEngineDialog>[0], 'engine' | 'title' | 'description' | 'hint'>) {
  return (
    <AddOAuthEngineDialog
      {...props}
      engine="antigravity"
      title="添加 Antigravity 账号"
      description="登录 Google 账号并授权后，把浏览器跳转的 localhost 地址完整粘贴回来。授权范围包含 Cloud AI Companion。"
      hint="直接粘贴 Google OAuth 的 refresh_token（需包含 cloud-platform 等 Antigravity 权限范围）。"
    />
  )
}

/** Kiro：粘贴 refresh_token（Kiro IDE 登录态提取）。 */export function AddKiroDialog({ open, onOpenChange, onDone }: { open: boolean; onOpenChange: (o: boolean) => void; onDone: () => void }) {
  const [token, setToken] = useState('')
  const [email, setEmail] = useState('')
  const [label, setLabel] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    if (open) {
      setToken('')
      setEmail('')
      setLabel('')
    }
  }, [open])
  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!token.trim()) return
    setBusy(true)
    try {
      await api('/admin/api/accounts', {
        method: 'POST',
        json: { engine: 'kiro', label: label.trim(), email: email.trim(), credentials: { refresh_token: token.trim(), email: email.trim() } },
      })
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
            <DialogTitle className="text-[15px]">添加 Kiro 账号</DialogTitle>
            <DialogDescription className="text-[13px]">在 Kiro IDE 完成登录后，从 ~/.aws/sso/cache/kiro-auth-token.json 中复制 refreshToken 字段。注意：Kiro 风控较严，建议少量账号自用。</DialogDescription>
          </DialogHeader>
          <Field label="refresh_token" htmlFor="kiro-rt">
            <Textarea id="kiro-rt" rows={3} required autoFocus spellCheck={false} value={token} onChange={(e) => setToken(e.target.value)} className="font-mono text-[11px]" />
          </Field>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="名称（可选）" htmlFor="kiro-label">
              <Input id="kiro-label" value={label} onChange={(e) => setLabel(e.target.value)} placeholder="留空使用邮箱" />
            </Field>
            <Field label="邮箱（可选）" htmlFor="kiro-email">
              <Input id="kiro-email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="user@example.com" />
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

/** WorkBuddy：设备授权轮询（扫码/浏览器登录后自动入池）。 */
export function AddWorkBuddyDialog({ open, onOpenChange, onDone }: { open: boolean; onOpenChange: (o: boolean) => void; onDone: () => void }) {
  const [session, setSession] = useState<OAuthStartResp | null>(null)
  const [pasteToken, setPasteToken] = useState('')
  const [label, setLabel] = useState('')
  const [busy, setBusy] = useState(false)
  const [polling, setPolling] = useState(false)
  const timer = useRef<ReturnType<typeof setInterval> | null>(null)

  useEffect(() => {
    if (!open) {
      if (timer.current) clearInterval(timer.current)
      timer.current = null
      setSession(null)
      setPasteToken('')
      setLabel('')
      setPolling(false)
    }
  }, [open])

  const poll = (sessionID: string) => {
    if (timer.current) clearInterval(timer.current)
    setPolling(true)
    timer.current = setInterval(async () => {
      try {
        const resp = await api<{ pending?: boolean; error?: string }>(`/admin/api/oauth/workbuddy/poll`, {
          method: 'POST',
          json: { session_id: sessionID },
        })
        if (resp && resp.pending) return // 等待登录，继续轮询
        if (timer.current) clearInterval(timer.current)
        timer.current = null
        setPolling(false)
        toast.success('账号已添加')
        onOpenChange(false)
        onDone()
      } catch (e) {
        if (timer.current) clearInterval(timer.current)
        timer.current = null
        setPolling(false)
        toast.error((e as Error).message)
      }
    }, 3000)
  }

  const startAuth = async () => {
    setBusy(true)
    try {
      const resp = await api<OAuthStartResp>('/admin/api/oauth/workbuddy/authorize', { method: 'POST', json: {} })
      setSession(resp)
      window.open(resp.auth_url, '_blank')
      poll(resp.session_id)
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const pasteSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!pasteToken.trim()) return
    setBusy(true)
    try {
      await api('/admin/api/accounts', {
        method: 'POST',
        json: { engine: 'workbuddy', label: label.trim(), credentials: { token: pasteToken.trim() } },
      })
      toast.success('账号已添加')
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
        <form onSubmit={pasteSubmit} className="space-y-4">
          <DialogHeader>
            <DialogTitle className="text-[15px]">添加 WorkBuddy / CodeBuddy 账号</DialogTitle>
            <DialogDescription className="text-[13px]">推荐扫码授权：在打开的页面登录 CodeBuddy 账号，本弹窗自动完成入池；也可以直接粘贴 accessToken。</DialogDescription>
          </DialogHeader>
          {!session ? (
            <Button type="button" onClick={startAuth} disabled={busy} className="w-full">
              {busy ? <Loader2 className="animate-spin" /> : null} 开始扫码授权
            </Button>
          ) : (
            <>
              <OAuthLinkBlock engine="workbuddy" url={session.auth_url} />
              <div className="flex items-center gap-2 text-[12px] text-muted-foreground">
                {polling && <Loader2 className="h-3.5 w-3.5 animate-spin" />} {polling ? '等待授权完成…' : '已停止轮询'}
              </div>
            </>
          )}
          <div className="space-y-2 border-t pt-3">
            <Field label="或直接粘贴 accessToken" htmlFor="wb-token">
              <Textarea id="wb-token" rows={2} spellCheck={false} value={pasteToken} onChange={(e) => setPasteToken(e.target.value)} className="font-mono text-[11px]" placeholder="Authorization: Bearer 后面的 JWT" />
            </Field>
            <Field label="名称（可选）" htmlFor="wb-label">
              <Input id="wb-label" value={label} onChange={(e) => setLabel(e.target.value)} placeholder="留空使用账号邮箱" />
            </Field>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button type="submit" disabled={busy || !pasteToken.trim()}>
              添加
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

/** 订阅引擎更新凭据弹窗（refresh_token / token）。 */
export function SubCredsDialog({
  account,
  onOpenChange,
  onDone,
}: {
  account: { id: number; label: string; engine: string } | null
  onOpenChange: (o: boolean) => void
  onDone: () => void
}) {
  const isTokenEngine = account?.engine === 'workbuddy'
  const [value, setValue] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    if (account) setValue('')
  }, [account])
  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!account || !value.trim()) return
    setBusy(true)
    try {
      const credentials = isTokenEngine ? { token: value.trim() } : { refresh_token: value.trim() }
      await api(`/admin/api/accounts/${account.id}/credentials`, { method: 'PUT', json: { credentials } })
      toast.success('凭据已更新，账号重新初始化中')
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
      <DialogContent className="sm:max-w-[480px]">
        <form onSubmit={submit} className="space-y-4">
          <DialogHeader>
            <DialogTitle className="text-[15px]">更新凭据</DialogTitle>
            <DialogDescription className="text-[13px]">{account?.label} · 旧凭据会被替换，账号将重新初始化。</DialogDescription>
          </DialogHeader>
          <Field label={isTokenEngine ? 'accessToken' : 'refresh_token'} htmlFor="sub-cred">
            <Textarea id="sub-cred" rows={3} required autoFocus spellCheck={false} value={value} onChange={(e) => setValue(e.target.value)} className="font-mono text-[11px]" />
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
