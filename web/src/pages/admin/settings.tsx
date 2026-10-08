import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { ArrowUpCircle, RefreshCw, Save } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { PageHeader, Section, KV } from '@/components/page'
import { BlockSkeleton, ErrorState } from '@/components/states'
import { Pill } from '@/components/status'
import { Field } from '@/components/fields'
import { useConfirm } from '@/components/confirm'
import { ApiError, api } from '@/lib/api'
import type { PlatformSettings, UpgradeState, VersionInfo } from '@/lib/types'
import { cn } from '@/lib/utils'

const busyPhases = ['checking', 'starting', 'upgrading', 'restarting']
const phaseText: Record<string, string> = {
  idle: '等待检查',
  checking: '正在检查并下载镜像',
  ready: '有可用更新',
  up_to_date: '已是最新版本',
  starting: '正在启动升级任务',
  upgrading: '正在切换容器',
  restarting: '正在等待新版本就绪',
  completed: '升级成功',
  rolled_back: '已恢复旧版本',
  failed: '任务失败',
}

export default function AdminSettings() {
  return (
    <>
      <PageHeader title="设置" description="平台运营规则、版本与在线升级。" />
      <div className="grid max-w-4xl gap-4">
        <PlatformForm />
        <UpgradePanel />
      </div>
    </>
  )
}

function Row({ title, description, children }: { title: string; description?: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-2 px-4 py-3.5 sm:flex-row sm:items-center sm:justify-between sm:gap-6">
      <div className="min-w-0">
        <div className="text-[13px] font-medium">{title}</div>
        {description && <p className="mt-0.5 text-xs leading-relaxed text-muted-foreground">{description}</p>}
      </div>
      <div className="shrink-0 sm:w-56 sm:text-right">{children}</div>
    </div>
  )
}

function PlatformForm() {
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['admin', 'settings'], queryFn: () => api<{ settings: PlatformSettings }>('/admin/api/settings') })
  const [draft, setDraft] = useState<PlatformSettings | null>(null)
  const [saving, setSaving] = useState(false)
  const s = draft || q.data?.settings
  const dirty = !!draft && JSON.stringify(draft) !== JSON.stringify(q.data?.settings)
  const set = <K extends keyof PlatformSettings>(k: K, v: PlatformSettings[K]) => s && setDraft({ ...s, [k]: v })

  const save = async () => {
    if (!draft) return
    setSaving(true)
    try {
      const res = await api<{ settings: PlatformSettings }>('/admin/api/settings', { method: 'PUT', json: draft })
      qc.setQueryData(['admin', 'settings'], res)
      qc.invalidateQueries({ queryKey: ['public-config'] })
      setDraft(null)
      toast.success('设置已保存')
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  if (q.isError) return <Section title="平台设置"><ErrorState error={q.error} onRetry={() => q.refetch()} /></Section>
  if (!s) return <Section title="平台设置"><div className="p-4"><BlockSkeleton className="h-72" /></div></Section>
  const intInput = (k: 'signup_bonus' | 'max_keys_per_user' | 'video_tokens_per_second') => (
    <Input inputMode="numeric" value={String(s[k])} onChange={(e) => set(k, Number(e.target.value.replace(/[^\d]/g, '')) || 0)} className="text-right tabular-nums sm:w-full" />
  )
  return (
    <Section
      title="平台设置"
      description="修改后点击保存立即生效，无需重启。"
      actions={
        <div className="flex items-center gap-2">
          {dirty && (
            <Button variant="ghost" size="sm" onClick={() => setDraft(null)}>
              放弃修改
            </Button>
          )}
          <Button size="sm" onClick={save} disabled={!dirty || saving}>
            <Save /> 保存
          </Button>
        </div>
      }
    >
      <div className="divide-y">
        <Row title="开放注册" description="关闭时只能由管理员创建用户；已注册用户不受影响。">
          <Switch checked={s.registration_open} onCheckedChange={(v) => set('registration_open', v)} aria-label="开放注册" />
        </Row>
        <Row title="注册赠送（Token）" description="新用户注册时自动到账的余额，0 表示不赠送。">
          {intInput('signup_bonus')}
        </Row>
        <Row title="新用户默认倍率" description="新注册用户的用户倍率；已有用户在用户详情里单独修改。">
          <Input inputMode="decimal" value={String(s.default_user_multiplier)} onChange={(e) => set('default_user_multiplier', Number(e.target.value) || 0)} className="text-right tabular-nums sm:w-full" />
        </Row>
        <Row title="每个用户最多密钥数">{intInput('max_keys_per_user')}</Row>
        <Row title="视频每秒计费 Token" description="用户密钥调用 /v1/videos 时按秒数 × 该值扣费；0 表示不允许用户密钥生成视频。">
          {intInput('video_tokens_per_second')}
        </Row>
        <Row title="允许用户密钥使用不计量接口" description="Gemini 原生接口等无法按 Token 计费的路由。默认关闭以防止绕过余额。">
          <Switch checked={s.user_unmetered_routes} onCheckedChange={(v) => set('user_unmetered_routes', v)} aria-label="允许不计量接口" />
        </Row>
        <div className="grid gap-3 px-4 py-3.5 sm:grid-cols-2">
          <Field label="站点名称" htmlFor="site-name" hint="显示在登录页与侧边栏">
            <Input id="site-name" value={s.site_name} maxLength={40} onChange={(e) => set('site_name', e.target.value)} />
          </Field>
          <Field label="控制台公告" htmlFor="announcement" className="sm:col-span-2" hint="显示在用户控制台首页，留空不显示。">
            <Textarea id="announcement" rows={3} value={s.announcement} maxLength={2000} onChange={(e) => set('announcement', e.target.value)} />
          </Field>
        </div>
      </div>
    </Section>
  )
}

function UpgradePanel() {
  const qc = useQueryClient()
  const confirm = useConfirm()
  const version = useQuery({ queryKey: ['admin', 'version'], queryFn: () => api<VersionInfo>('/admin/api/version') })
  const [disconnected, setDisconnected] = useState(false)
  const up = useQuery({
    queryKey: ['admin', 'upgrade'],
    queryFn: async () => {
      try {
        const res = await api<UpgradeState>('/admin/api/upgrade')
        const prev = qc.getQueryData<UpgradeState>(['admin', 'upgrade'])
        setDisconnected(false)
        if (prev && busyPhases.includes(prev.phase || '') && res.phase === 'completed' && res.current_id === res.target_id) setTimeout(() => location.reload(), 500)
        return res
      } catch (e) {
        const prev = qc.getQueryData<UpgradeState>(['admin', 'upgrade'])
        if (!(e instanceof ApiError && e.status) && prev && busyPhases.includes(prev.phase || '')) {
          setDisconnected(true)
          return prev
        }
        throw e
      }
    },
    refetchInterval: (query) => (busyPhases.includes(query.state.data?.phase || '') ? 2000 : 30_000),
  })
  const [busy, setBusy] = useState(false)
  const u = up.data
  const check = async () => {
    setBusy(true)
    try {
      await api('/admin/api/upgrade/check', { method: 'POST' })
      qc.setQueryData(['admin', 'upgrade'], { ...u, phase: 'checking', message: '正在拉取最新镜像，服务继续运行。', target_id: '', target_version: '' })
      up.refetch()
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      setBusy(false)
    }
  }
  const start = async () => {
    if (!(await confirm({ title: '升级到最新镜像？', description: `将升级到 ${u?.target_version || '已检查的最新版本'}。服务会短暂中断，失败时自动恢复旧容器，完成后页面自动刷新。`, confirmText: '开始升级' }))) return
    setBusy(true)
    try {
      await api('/admin/api/upgrade', { method: 'POST', signal: AbortSignal.timeout(20000) })
      up.refetch()
    } catch (e) {
      if (e instanceof ApiError && e.status) toast.error(e.message)
      else setDisconnected(true)
    } finally {
      setBusy(false)
    }
  }
  const phase = u?.phase || 'idle'
  const failed = phase === 'failed'
  const v = version.data
  return (
    <Section
      title="版本与升级"
      actions={<Pill tone={failed ? 'danger' : phase === 'ready' ? 'primary' : phase === 'completed' || phase === 'up_to_date' ? 'success' : busyPhases.includes(phase) ? 'warning' : 'muted'}>{disconnected ? '重连中' : phaseText[phase] || phase}</Pill>}
    >
      <div className="space-y-4 p-4">
        {v && (
          <KV
            items={[
              ['当前版本', <span className="font-mono text-xs">{v.version}{v.modified ? '（含未提交修改）' : ''}</span>],
              ['提交', <span className="font-mono text-xs">{v.commit ? v.commit.slice(0, 12) : '—'}</span>],
              ['构建时间', v.build_time || '—'],
              ['Go', v.go_version],
            ]}
          />
        )}
        {up.isLoading ? (
          <BlockSkeleton className="h-24" />
        ) : up.isError ? (
          <ErrorState error={up.error} onRetry={() => up.refetch()} />
        ) : (
          u && (
            <>
              <div className="grid gap-3 sm:grid-cols-2">
                <div className="rounded-md border p-3">
                  <div className="text-[11px] text-muted-foreground">运行中的镜像</div>
                  <div className="mt-0.5 text-[13px] font-medium">{u.current_version || (u.current_id ? '本地构建' : '未获取')}</div>
                  <code className="block truncate text-[11px] text-muted-foreground">{u.current_id || ''}</code>
                </div>
                <div className="rounded-md border p-3">
                  <div className="text-[11px] text-muted-foreground">最新镜像</div>
                  <div className="mt-0.5 text-[13px] font-medium">{u.target_version || (u.target_id ? '镜像已获取' : '尚未检查')}</div>
                  <code className="block truncate text-[11px] text-muted-foreground">{u.target_id || ''}</code>
                </div>
              </div>
              <div className={cn('rounded-md border p-3 text-xs', failed ? 'border-destructive/30 bg-destructive/[0.06]' : 'bg-white/[0.02]')}>
                <div className={cn('font-medium', failed ? 'text-[#ff7a7e]' : 'text-foreground')}>{disconnected ? '服务重启中，正在重新连接…' : phaseText[phase] || phase}</div>
                <p className="mt-1 leading-relaxed break-words whitespace-pre-line text-muted-foreground">{u.message || '镜像：' + (u.image || 'ghcr.io/plunjoin/web2api:latest')}</p>
              </div>
              {!u.enabled && u.reason && u.reason !== u.message && (
                <div className="rounded-md border border-warning/20 bg-warning/[0.06] p-3 text-xs leading-relaxed whitespace-pre-line text-warning">{u.reason}</div>
              )}
              <div className="flex flex-wrap gap-2">
                <Button variant="outline" size="sm" onClick={check} disabled={!u.enabled || busy || busyPhases.includes(phase)}>
                  <RefreshCw /> 检查更新
                </Button>
                <Button size="sm" onClick={start} disabled={!u.enabled || phase !== 'ready' || busy}>
                  <ArrowUpCircle /> 一键升级
                </Button>
              </div>
            </>
          )
        )}
      </div>
    </Section>
  )
}
