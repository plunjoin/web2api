import { Link } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { Activity, Coins, KeyRound, Plus, Server, Ticket, Users, Wallet } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { PageHeader, Section } from '@/components/page'
import { StatStrip } from '@/components/stat'
import { BlockSkeleton, ErrorState, EmptyState } from '@/components/states'
import { RankBars, UsageChart } from '@/components/usage-chart'
import { AccountStatus, accountHealth } from '@/components/account-status'
import { api } from '@/lib/api'
import { compact, duration, num } from '@/lib/format'
import type { Account, UsagePoint, VersionInfo } from '@/lib/types'

interface Overview {
  platform: {
    users: number
    users_enabled: number
    balance_total: number
    recharged_30d: number
    consumed_30d: number
    new_users_7d: number
    active_users_7d: number
    unused_codes: number
    unused_code_value: number
  }
  accounts_total: number
  accounts_ready: number
  engine_a_ready: number
  engine_b_ready: number
  keys_total: number
  requests_24h: number
  metered_requests_24h?: number
  keys_limited: number
  keys_exhausted: number
  tokens_24h: number
  charged_tokens_24h: number
  version: VersionInfo
  uptime_seconds: number
}

interface BreakdownRow {
  model: string
  key_name: string
  key_masked: string
  requests: number
  charged_tokens: number
}

export default function AdminOverview() {
  const ov = useQuery({ queryKey: ['admin', 'overview'], queryFn: () => api<Overview>('/admin/api/overview'), refetchInterval: 30_000 })
  const series = useQuery({ queryKey: ['admin', 'series', 14], queryFn: () => api<{ points: UsagePoint[] }>('/admin/api/usage/timeseries?days=14') })
  const usage = useQuery({ queryKey: ['admin', 'usage', 7], queryFn: () => api<{ breakdown: BreakdownRow[] }>('/admin/api/usage?days=7') })
  const accounts = useQuery({ queryKey: ['admin', 'accounts'], queryFn: () => api<{ accounts: Account[] }>('/admin/api/accounts') })
  const d = ov.data
  const p = d?.platform
  const accs = accounts.data?.accounts
  const accUp = accs ? accs.filter((a) => accountHealth(a) === 'up').length : (d?.accounts_ready ?? 0)
  const accDown = accs ? accs.filter((a) => accountHealth(a) === 'down').length : 0

  const byModel = new Map<string, number>()
  const byKey = new Map<string, number>()
  for (const r of usage.data?.breakdown || []) {
    byModel.set(r.model || '未知', (byModel.get(r.model || '未知') || 0) + r.charged_tokens)
    const k = r.key_name || r.key_masked || '未知'
    byKey.set(k, (byKey.get(k) || 0) + r.charged_tokens)
  }
  const rank = (m: Map<string, number>) =>
    [...m.entries()]
      .sort((a, b) => b[1] - a[1])
      .slice(0, 6)
      .map(([label, value]) => ({ label, value }))

  return (
    <>
      <PageHeader
        title="概览"
        description={d ? `${d.version.version} · 已运行 ${duration(d.uptime_seconds)}` : '平台与网关的整体运行情况'}
        actions={
          <>
            <Button variant="outline" asChild>
              <Link to="/admin/codes?new=1">
                <Ticket /> 生成兑换码
              </Link>
            </Button>
            <Button asChild>
              <Link to="/admin/users?new=1">
                <Plus /> 新建用户
              </Link>
            </Button>
          </>
        }
      />
      {ov.isError ? (
        <Section>
          <ErrorState error={ov.error} onRetry={() => ov.refetch()} />
        </Section>
      ) : (
        <div className="space-y-4">
          <StatStrip
            loading={ov.isLoading}
            items={[
              { label: '上游账号', icon: <Server />, value: `${accUp} / ${d?.accounts_total ?? 0}`, tone: d && d.accounts_total > 0 && accUp === 0 ? 'danger' : accDown ? 'warning' : 'default', hint: accDown ? `${accDown} 个异常，需要检查` : '可用 / 全部' },
              { label: '24 小时请求', icon: <Activity />, value: num(d?.metered_requests_24h ?? d?.requests_24h), hint: `计费 ${compact(d?.charged_tokens_24h)} Token` },
              { label: 'API 密钥', icon: <KeyRound />, value: num(d?.keys_total), hint: d?.keys_exhausted ? `${d.keys_exhausted} 个额度用尽` : `${d?.keys_limited ?? 0} 个设置了额度`, tone: d?.keys_exhausted ? 'warning' : 'default' },
              { label: '用户', icon: <Users />, value: num(p?.users), hint: `7 天新增 ${p?.new_users_7d ?? 0} · 活跃 ${p?.active_users_7d ?? 0}` },
            ]}
          />
          <StatStrip
            loading={ov.isLoading}
            items={[
              { label: '用户余额合计', icon: <Wallet />, value: compact(p?.balance_total), hint: '尚未消耗的 Token' },
              { label: '30 天充值', icon: <Coins />, value: compact(p?.recharged_30d), tone: 'success', hint: '兑换 + 调整 + 赠送' },
              { label: '30 天用户消耗', icon: <Activity />, value: compact(p?.consumed_30d) },
              { label: '未使用兑换码', icon: <Ticket />, value: num(p?.unused_codes), hint: `面值合计 ${compact(p?.unused_code_value)}` },
            ]}
          />
          <div className="grid gap-4 lg:grid-cols-3">
            <Section title="近 14 天计费 Token" description="全部密钥（含无归属的管理员密钥）" className="lg:col-span-2">
              <div className="px-2 pt-3 pb-2">{series.isLoading ? <BlockSkeleton className="h-[220px]" /> : <UsageChart points={series.data?.points || []} />}</div>
            </Section>
            <Section
              title="上游账号"
              actions={
                <Button variant="ghost" size="sm" asChild>
                  <Link to="/admin/accounts">管理</Link>
                </Button>
              }
            >
              {accounts.isLoading ? (
                <div className="p-4">
                  <BlockSkeleton className="h-32" />
                </div>
              ) : !accounts.data?.accounts.length ? (
                <EmptyState
                  icon={<Server />}
                  title="还没有上游账号"
                  description="添加 Gemini 网页号或 AI Studio 号后才能处理请求。"
                  action={
                    <Button size="sm" asChild>
                      <Link to="/admin/accounts">添加账号</Link>
                    </Button>
                  }
                />
              ) : (
                <ul className="divide-y">
                  {accounts.data.accounts.slice(0, 8).map((a) => (
                    <li key={a.id} className="flex items-center justify-between gap-3 px-4 py-2 text-[13px]">
                      <div className="min-w-0">
                        <div className="truncate">{a.label}</div>
                        <div className="text-[11px] text-muted-foreground">{a.engine === 'a' ? 'Gemini 网页' : 'AI Studio'} · {a.live?.models ?? 0} 个模型</div>
                      </div>
                      <AccountStatus account={a} />
                    </li>
                  ))}
                </ul>
              )}
            </Section>
          </div>
          <div className="grid gap-4 lg:grid-cols-2">
            <Section title="模型消耗排行" description="近 7 天计费 Token">
              {usage.isLoading ? <div className="p-4"><BlockSkeleton className="h-32" /></div> : <RankBars items={rank(byModel)} empty="近 7 天没有请求" format={compact} />}
            </Section>
            <Section title="密钥消耗排行" description="近 7 天计费 Token">
              {usage.isLoading ? <div className="p-4"><BlockSkeleton className="h-32" /></div> : <RankBars items={rank(byKey)} empty="近 7 天没有请求" format={compact} />}
            </Section>
          </div>
        </div>
      )}
    </>
  )
}
