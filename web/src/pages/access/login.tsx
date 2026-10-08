import { useState } from 'react'
import { Link, Navigate, useNavigate, useSearchParams } from 'react-router'
import { useQueryClient } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { usePublicConfig } from '@/components/app-shell'
import { api } from '@/lib/api'
import { useAuth, isAdmin } from '@/lib/auth'
import { AuthLayout, FormError } from './auth-layout'

export default function LoginPage() {
  const { me, loading, login } = useAuth()
  const { data: config, isLoading } = usePublicConfig()
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  if (!loading && me) return <Navigate to={params.get('next') || (isAdmin(me) ? '/admin' : '/console')} replace />
  if (isLoading || loading) {
    return (
      <AuthLayout title="登录">
        <div className="space-y-3">
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-full" />
        </div>
      </AuthLayout>
    )
  }
  if (config && !config.initialized) return <SetupForm />

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setBusy(true)
    try {
      const user = await login(email, password)
      navigate(params.get('next') || (isAdmin(user) ? '/admin' : '/console'), { replace: true })
    } catch (err) {
      setError(err instanceof Error ? err.message : '登录失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <AuthLayout
      title={`登录到 ${config?.site_name || 'web2api'}`}
      subtitle="使用邮箱和密码登录控制台"
      footer={
        config?.registration_open ? (
          <>
            还没有账号？
            <Link to="/register" className="ml-1 text-foreground underline-offset-4 hover:underline">
              注册
            </Link>
          </>
        ) : (
          '需要账号请联系管理员开通'
        )
      }
    >
      <form className="space-y-3.5" onSubmit={submit}>
        <div className="space-y-1.5">
          <Label htmlFor="email" className="text-xs">邮箱</Label>
          <Input id="email" type="email" autoComplete="email" autoFocus required value={email} onChange={(e) => setEmail(e.target.value)} placeholder="you@example.com" />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="password" className="text-xs">密码</Label>
          <Input id="password" type="password" autoComplete="current-password" required value={password} onChange={(e) => setPassword(e.target.value)} />
        </div>
        <FormError message={error} />
        <Button type="submit" className="w-full" disabled={busy}>
          {busy && <Loader2 className="animate-spin" />} 登录
        </Button>
      </form>
    </AuthLayout>
  )
}

function SetupForm() {
  const qc = useQueryClient()
  const { login } = useAuth()
  const navigate = useNavigate()
  const [form, setForm] = useState({ email: '', password: '', nickname: '', redis_url: 'redis://127.0.0.1:6379/0' })
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) => setForm({ ...form, [k]: e.target.value })
  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api('/admin/api/setup', { method: 'POST', json: form })
      await qc.invalidateQueries({ queryKey: ['public-config'] })
      await login(form.email, form.password)
      navigate('/admin', { replace: true })
    } catch (err) {
      setError(err instanceof Error ? err.message : '初始化失败')
    } finally {
      setBusy(false)
    }
  }
  return (
    <AuthLayout title="首次初始化" subtitle="创建管理员账号并连接 Redis（登录会话存储）">
      <form className="space-y-3.5" onSubmit={submit}>
        <div className="space-y-1.5">
          <Label htmlFor="s-email" className="text-xs">管理员邮箱</Label>
          <Input id="s-email" type="email" required value={form.email} onChange={set('email')} />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="s-nick" className="text-xs">昵称</Label>
          <Input id="s-nick" required value={form.nickname} onChange={set('nickname')} />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="s-pass" className="text-xs">密码</Label>
          <Input id="s-pass" type="password" required minLength={8} value={form.password} onChange={set('password')} />
          <p className="text-[11px] text-muted-foreground">至少 8 个字符，最多 72 字节</p>
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="s-redis" className="text-xs">Redis 连接地址</Label>
          <Input id="s-redis" required value={form.redis_url} onChange={set('redis_url')} className="font-mono text-xs" />
          <p className="text-[11px] text-muted-foreground">支持 redis:// 与 rediss://（TLS），保存前会验证读写权限</p>
        </div>
        <FormError message={error} />
        <Button type="submit" className="w-full" disabled={busy}>
          {busy && <Loader2 className="animate-spin" />} 完成初始化
        </Button>
      </form>
    </AuthLayout>
  )
}
