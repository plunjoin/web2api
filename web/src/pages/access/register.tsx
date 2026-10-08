import { useState } from 'react'
import { Link, Navigate, useNavigate } from 'react-router'
import { toast } from 'sonner'
import { Gift, Loader2, Lock } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { usePublicConfig } from '@/components/app-shell'
import { EmptyState } from '@/components/states'
import { useAuth } from '@/lib/auth'
import { num } from '@/lib/format'
import { AuthLayout, FormError } from './auth-layout'

export default function RegisterPage() {
  const { me, register } = useAuth()
  const { data: config, isLoading } = usePublicConfig()
  const navigate = useNavigate()
  const [form, setForm] = useState({ email: '', nickname: '', password: '', confirm: '' })
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  if (me) return <Navigate to="/" replace />
  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) => setForm({ ...form, [k]: e.target.value })

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    if (form.password !== form.confirm) {
      setError('两次输入的密码不一致')
      return
    }
    setBusy(true)
    try {
      await register(form.email, form.password, form.nickname)
      toast.success('注册成功，欢迎使用')
      navigate('/console', { replace: true })
    } catch (err) {
      setError(err instanceof Error ? err.message : '注册失败')
    } finally {
      setBusy(false)
    }
  }

  const footer = (
    <>
      已有账号？
      <Link to="/login" className="ml-1 text-foreground underline-offset-4 hover:underline">
        登录
      </Link>
    </>
  )
  if (isLoading) {
    return (
      <AuthLayout title="创建账号" footer={footer}>
        <Skeleton className="h-48 w-full" />
      </AuthLayout>
    )
  }
  if (!config?.registration_open) {
    return (
      <AuthLayout title="暂未开放注册" footer={footer}>
        <EmptyState icon={<Lock />} title="管理员关闭了自助注册" description="如需账号，请联系管理员为你开通。" className="py-6" />
      </AuthLayout>
    )
  }
  return (
    <AuthLayout title="创建账号" subtitle="注册后即可创建 API 密钥并按 Token 计费使用" footer={footer}>
      {config.signup_bonus > 0 && (
        <div className="mb-4 flex items-center gap-2 rounded-md border border-primary/20 bg-primary/10 px-3 py-2 text-xs text-[#b9bef8]">
          <Gift className="size-3.5" /> 注册即送 {num(config.signup_bonus)} Token 余额
        </div>
      )}
      <form className="space-y-3.5" onSubmit={submit}>
        <div className="space-y-1.5">
          <Label htmlFor="r-email" className="text-xs">邮箱</Label>
          <Input id="r-email" type="email" autoComplete="email" required autoFocus value={form.email} onChange={set('email')} placeholder="you@example.com" />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="r-nick" className="text-xs">昵称 <span className="text-muted-foreground">（可选）</span></Label>
          <Input id="r-nick" value={form.nickname} maxLength={64} onChange={set('nickname')} />
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div className="space-y-1.5">
            <Label htmlFor="r-pass" className="text-xs">密码</Label>
            <Input id="r-pass" type="password" autoComplete="new-password" required minLength={8} value={form.password} onChange={set('password')} />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="r-confirm" className="text-xs">确认密码</Label>
            <Input id="r-confirm" type="password" autoComplete="new-password" required minLength={8} value={form.confirm} onChange={set('confirm')} />
          </div>
        </div>
        <p className="text-[11px] text-muted-foreground">密码至少 8 个字符</p>
        <FormError message={error} />
        <Button type="submit" className="w-full" disabled={busy}>
          {busy && <Loader2 className="animate-spin" />} 创建账号
        </Button>
      </form>
    </AuthLayout>
  )
}
