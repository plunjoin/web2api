import { useState } from 'react'
import { toast } from 'sonner'
import { Dices } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Field, MiniSelect } from '@/components/fields'
import { copyText } from '@/components/copy'
import { api } from '@/lib/api'
import { num } from '@/lib/format'
import type { User } from '@/lib/types'
import { cn } from '@/lib/utils'

/** 打开时重置表单：用“上一次 open 状态”比较，避免 effect 里 setState。 */
function useOnOpen(open: boolean, reset: () => void) {
  const [last, setLast] = useState(false)
  if (open !== last) {
    setLast(open)
    if (open) reset()
  }
}

function randomPassword() {
  const alphabet = 'ABCDEFGHJKMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789'
  const buf = new Uint32Array(14)
  crypto.getRandomValues(buf)
  return Array.from(buf, (n) => alphabet[n % alphabet.length]).join('')
}

const roleOptions = [
  { value: 'user', label: '普通用户' },
  { value: 'admin', label: '管理员' },
]

export function CreateUserDialog({ open, onOpenChange, onCreated }: { open: boolean; onOpenChange: (o: boolean) => void; onCreated: (u: User) => void }) {
  const [f, setF] = useState({ email: '', nickname: '', password: '', role: 'user', balance: '', multiplier: '', note: '' })
  const [busy, setBusy] = useState(false)
  useOnOpen(open, () => setF({ email: '', nickname: '', password: randomPassword(), role: 'user', balance: '', multiplier: '', note: '' }))
  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      const body: Record<string, unknown> = { email: f.email.trim(), nickname: f.nickname.trim(), password: f.password, role: f.role, note: f.note.trim() }
      if (f.balance) body.balance = Number(f.balance)
      if (f.multiplier) body.multiplier = Number(f.multiplier)
      const res = await api<{ user: User }>('/admin/api/users', { method: 'POST', json: body })
      toast.success('用户已创建', {
        description: '初始密码只显示这一次',
        action: { label: '复制密码', onClick: () => copyText(f.password, '密码已复制') },
        duration: 10000,
      })
      onOpenChange(false)
      onCreated(res.user)
    } catch (err) {
      toast.error((err as Error).message)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[480px]">
        <form onSubmit={submit} className="space-y-4">
          <DialogHeader>
            <DialogTitle className="text-[15px]">新建用户</DialogTitle>
            <DialogDescription className="text-[13px]">管理员直接创建的账号不受“开放注册”开关影响。</DialogDescription>
          </DialogHeader>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="邮箱" htmlFor="u-email" className="sm:col-span-2">
              <Input id="u-email" type="email" required autoFocus value={f.email} onChange={(e) => setF({ ...f, email: e.target.value })} placeholder="user@example.com" />
            </Field>
            <Field label="昵称" htmlFor="u-nick">
              <Input id="u-nick" value={f.nickname} onChange={(e) => setF({ ...f, nickname: e.target.value })} placeholder="可选" />
            </Field>
            <Field label="角色">
              <MiniSelect value={f.role} onChange={(v) => setF({ ...f, role: v })} options={roleOptions} className="h-8 w-full text-[13px]" />
            </Field>
            <Field label="初始密码" htmlFor="u-pass" className="sm:col-span-2" hint="至少 8 个字符。创建后请通过安全渠道告知用户。">
              <div className="flex gap-2">
                <Input id="u-pass" required value={f.password} onChange={(e) => setF({ ...f, password: e.target.value })} className="font-mono" autoComplete="off" />
                <Button type="button" variant="outline" size="icon" onClick={() => setF({ ...f, password: randomPassword() })} aria-label="随机生成">
                  <Dices />
                </Button>
              </div>
            </Field>
            <Field label="初始余额" htmlFor="u-balance" hint="会记一条“管理员调整”流水">
              <Input id="u-balance" inputMode="numeric" value={f.balance} onChange={(e) => setF({ ...f, balance: e.target.value.replace(/[^\d]/g, '') })} placeholder="0" />
            </Field>
            <Field label="用户倍率" htmlFor="u-mult" hint="留空使用平台默认值">
              <Input id="u-mult" inputMode="decimal" value={f.multiplier} onChange={(e) => setF({ ...f, multiplier: e.target.value })} placeholder="默认" />
            </Field>
            <Field label="备注" htmlFor="u-note" className="sm:col-span-2">
              <Input id="u-note" value={f.note} onChange={(e) => setF({ ...f, note: e.target.value })} placeholder="仅管理员可见" />
            </Field>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button type="submit" disabled={busy}>
              创建
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export function EditUserDialog({ user, onOpenChange, onSaved }: { user: User | null; onOpenChange: (o: boolean) => void; onSaved: () => void }) {
  const [f, setF] = useState({ nickname: '', role: 'user', multiplier: '', note: '' })
  const [busy, setBusy] = useState(false)
  useOnOpen(!!user, () => user && setF({ nickname: user.nickname, role: user.role, multiplier: String(user.multiplier), note: user.note }))
  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!user) return
    const m = Number(f.multiplier)
    if (!(m > 0)) {
      toast.error('倍率必须大于 0')
      return
    }
    setBusy(true)
    try {
      await api(`/admin/api/users/${user.id}`, { method: 'PATCH', json: { nickname: f.nickname, role: f.role, multiplier: m, note: f.note } })
      toast.success('已保存')
      onOpenChange(false)
      onSaved()
    } catch (err) {
      toast.error((err as Error).message)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Dialog open={!!user} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[440px]">
        <form onSubmit={submit} className="space-y-4">
          <DialogHeader>
            <DialogTitle className="text-[15px]">编辑用户</DialogTitle>
            <DialogDescription className="text-[13px]">{user?.email}</DialogDescription>
          </DialogHeader>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="昵称" htmlFor="e-nick">
              <Input id="e-nick" value={f.nickname} onChange={(e) => setF({ ...f, nickname: e.target.value })} />
            </Field>
            <Field label="角色" hint={f.role !== user?.role ? '修改角色会让该用户重新登录' : undefined}>
              <MiniSelect value={f.role} onChange={(v) => setF({ ...f, role: v })} options={roleOptions} className="h-8 w-full text-[13px]" />
            </Field>
            <Field label="用户倍率" htmlFor="e-mult" className="sm:col-span-2" hint="叠乘在模型倍率与 Key 倍率之上，只影响之后的请求。">
              <Input id="e-mult" inputMode="decimal" value={f.multiplier} onChange={(e) => setF({ ...f, multiplier: e.target.value })} />
            </Field>
            <Field label="备注" htmlFor="e-note" className="sm:col-span-2">
              <Textarea id="e-note" rows={2} value={f.note} onChange={(e) => setF({ ...f, note: e.target.value })} />
            </Field>
          </div>
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

export function AdjustBalanceDialog({ user, onOpenChange, onSaved }: { user: User | null; onOpenChange: (o: boolean) => void; onSaved: () => void }) {
  const [dir, setDir] = useState<'add' | 'sub'>('add')
  const [amount, setAmount] = useState('')
  const [note, setNote] = useState('')
  const [busy, setBusy] = useState(false)
  useOnOpen(!!user, () => {
    setDir('add')
    setAmount('')
    setNote('')
  })
  const n = Number(amount) || 0
  const delta = dir === 'add' ? n : -n
  const after = (user?.balance ?? 0) + delta
  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!user || !n) return
    setBusy(true)
    try {
      await api(`/admin/api/users/${user.id}/balance`, { method: 'POST', json: { amount: delta, note: note.trim() } })
      toast.success(`已${dir === 'add' ? '增加' : '扣减'} ${num(n)} Token`)
      onOpenChange(false)
      onSaved()
    } catch (err) {
      toast.error((err as Error).message)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Dialog open={!!user} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[420px]">
        <form onSubmit={submit} className="space-y-4">
          <DialogHeader>
            <DialogTitle className="text-[15px]">调整余额</DialogTitle>
            <DialogDescription className="text-[13px]">
              {user?.email} · 当前余额 <span className="text-foreground tabular-nums">{num(user?.balance)}</span>
            </DialogDescription>
          </DialogHeader>
          <Tabs value={dir} onValueChange={(v) => setDir(v as 'add' | 'sub')}>
            <TabsList className="grid w-full grid-cols-2">
              <TabsTrigger value="add">增加</TabsTrigger>
              <TabsTrigger value="sub">扣减</TabsTrigger>
            </TabsList>
          </Tabs>
          <Field label="数量（Token）" htmlFor="adj-amount">
            <Input id="adj-amount" autoFocus required inputMode="numeric" value={amount} onChange={(e) => setAmount(e.target.value.replace(/[^\d]/g, ''))} placeholder="例如 100000" className="tabular-nums" />
          </Field>
          <Field label="备注" htmlFor="adj-note" hint="必填，会写入流水，用户可见。">
            <Input id="adj-note" required value={note} maxLength={200} onChange={(e) => setNote(e.target.value)} placeholder="例如：线下充值 / 补偿故障" />
          </Field>
          <div className="flex items-center justify-between rounded-md border bg-white/[0.02] px-3 py-2 text-[13px]">
            <span className="text-muted-foreground">调整后余额</span>
            <span className={cn('font-medium tabular-nums', after < 0 && 'text-destructive')}>{num(after)}</span>
          </div>
          {after < 0 && <p className="text-xs text-[#ff7a7e]">扣减后余额不能为负数。</p>}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button type="submit" disabled={busy || !n || !note.trim() || after < 0}>
              确认{dir === 'add' ? '增加' : '扣减'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export function ResetPasswordDialog({ user, onOpenChange }: { user: User | null; onOpenChange: (o: boolean) => void }) {
  const [pw, setPw] = useState('')
  const [busy, setBusy] = useState(false)
  useOnOpen(!!user, () => setPw(randomPassword()))
  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!user) return
    setBusy(true)
    try {
      await api(`/admin/api/users/${user.id}/password`, { method: 'POST', json: { password: pw } })
      await copyText(pw, '新密码已复制')
      toast.success('密码已重置，该用户的所有登录已失效')
      onOpenChange(false)
    } catch (err) {
      toast.error((err as Error).message)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Dialog open={!!user} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[420px]">
        <form onSubmit={submit} className="space-y-4">
          <DialogHeader>
            <DialogTitle className="text-[15px]">重置密码</DialogTitle>
            <DialogDescription className="text-[13px]">{user?.email} 的所有登录会立即失效。确认后新密码会复制到剪贴板。</DialogDescription>
          </DialogHeader>
          <Field label="新密码" htmlFor="rp">
            <div className="flex gap-2">
              <Input id="rp" required value={pw} onChange={(e) => setPw(e.target.value)} className="font-mono" autoComplete="off" />
              <Button type="button" variant="outline" size="icon" onClick={() => setPw(randomPassword())} aria-label="随机生成">
                <Dices />
              </Button>
            </div>
          </Field>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button type="submit" disabled={busy || pw.length < 8}>
              重置并复制
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
