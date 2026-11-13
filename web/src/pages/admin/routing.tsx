import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { ChevronDown, ChevronUp, RotateCcw, Save, Shuffle } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { PageHeader, Section } from '@/components/page'
import { ErrorState, BlockSkeleton } from '@/components/states'
import { StatusDot } from '@/components/status'
import { api } from '@/lib/api'
import { engineLabel, type RoutingAccount, type RoutingSupplier, type RoutingView } from '@/lib/types'
import { cn } from '@/lib/utils'

type Draft = RoutingSupplier[]

function supplierLabel(s: RoutingSupplier): string {
  if (s.engine === 'custom') {
    return s.provider ? `第三方 Key · ${s.provider}` : '第三方 Key · 未识别地址'
  }
  return engineLabel(s.engine)
}

const flatten = (groups: Draft): number[] => groups.flatMap((g) => g.accounts.map((a) => a.id))

function sameOrder(a: number[], b: number[]): boolean {
  return a.length === b.length && a.every((v, i) => v === b[i])
}

function move<T>(list: T[], from: number, to: number): T[] {
  if (to < 0 || to >= list.length || from === to) return list
  const next = [...list]
  const [item] = next.splice(from, 1)
  next.splice(to, 0, item)
  return next
}

function AccountRow({ account, index, total, onMove }: {
  account: RoutingAccount; index: number; total: number; onMove: (from: number, to: number) => void
}) {
  const tone = !account.enabled ? 'muted' : account.ready ? 'success' : 'warning'
  const status = !account.enabled ? '已停用' : account.ready ? '可用' : account.status === 'initializing' ? '初始化中' : '不可用'
  return (
    <div className="flex items-center gap-3 border-t px-4 py-2.5 first:border-t-0">
      <span className="w-6 shrink-0 text-right text-xs tabular-nums text-muted-foreground">{index + 1}</span>
      <div className="min-w-0 flex-1">
        <div className="break-words text-sm [overflow-wrap:anywhere]">{account.label || `#${account.id}`}</div>
        <div className="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
          <StatusDot tone={tone}>{status}</StatusDot>
          <span>{account.models} 个模型</span>
          <span className="tabular-nums">#{account.id}</span>
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-1">
        <Button variant="ghost" size="icon" aria-label="上移" disabled={index === 0} onClick={() => onMove(index, index - 1)}>
          <ChevronUp />
        </Button>
        <Button variant="ghost" size="icon" aria-label="下移" disabled={index === total - 1} onClick={() => onMove(index, index + 1)}>
          <ChevronDown />
        </Button>
      </div>
    </div>
  )
}

export default function AdminRouting() {
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['admin', 'routing'], queryFn: () => api<RoutingView>('/admin/api/routing') })
  const [draft, setDraft] = useState<Draft | null>(null)
  const [affinity, setAffinity] = useState<RoutingView['affinity'] | null>(null)
  const [saving, setSaving] = useState(false)
  const groups = draft || q.data?.suppliers || []
  const savedOrder = useMemo(() => flatten(q.data?.suppliers || []), [q.data])
  const currentOrder = useMemo(() => flatten(groups), [groups])
  const affinityValue = affinity ?? q.data?.affinity ?? 'session'
  const dirty = !!draft && !sameOrder(savedOrder, currentOrder)
  // 未保存过顺序时也允许直接保存当前（默认）顺序，用它启用严格优先级。
  const canSave = dirty || (!q.data?.configured && (q.data?.suppliers?.length || 0) > 0)

  const update = (fn: (groups: Draft) => Draft) => {
    setDraft((prev) => fn(prev || (q.data?.suppliers || []).map((g) => ({ ...g, accounts: [...g.accounts] }))))
  }
  const moveSupplier = (from: number, to: number) => update((list) => move(list, from, to))
  const moveAccount = (groupIndex: number, from: number, to: number) =>
    update((list) => list.map((g, i) => (i === groupIndex ? { ...g, accounts: move(g.accounts, from, to) } : g)))

  const save = async (accounts: number[]) => {
    setSaving(true)
    try {
      const res = await api<RoutingView>('/admin/api/routing', { method: 'PUT', json: { accounts, affinity: affinityValue } })
      qc.setQueryData(['admin', 'routing'], res)
      setDraft(null)
      setAffinity(null)
      toast.success(accounts.length ? '路由顺序已保存，立即生效' : '已恢复默认顺序')
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  // 会话粘性单独保存，不触碰已保存的路由顺序。
  const saveAffinity = async (value: RoutingView['affinity']) => {
    setAffinity(value)
    try {
      const res = await api<RoutingView>('/admin/api/routing', { method: 'PUT', json: { affinity: value } })
      qc.setQueryData(['admin', 'routing'], res)
      setAffinity(null)
      toast.success(value === 'session' ? '已开启会话粘性' : '已关闭会话粘性')
    } catch (e) {
      setAffinity(null)
      toast.error((e as Error).message)
    }
  }

  if (q.isError) return <Section title="路由顺序"><ErrorState error={q.error} onRetry={() => q.refetch()} /></Section>
  if (!q.data) return <Section title="路由顺序"><div className="p-4"><BlockSkeleton className="h-72" /></div></Section>

  return (
    <>
      <PageHeader
        title="路由顺序"
        description="同一个模型有多个供应商/账号可用时，按这里的顺序从上到下尝试：当前供应商的账号全部失败后，才走下一个供应商。没保存过时按供应商、账号的添加顺序。"
        actions={
          <div className="flex items-center gap-2">
            <Button variant="outline" disabled={saving || !q.data.configured} onClick={() => save([])}>
              <RotateCcw /> 恢复默认
            </Button>
            <Button disabled={!canSave || saving} onClick={() => save(currentOrder)}>
              <Save /> {saving ? '保存中…' : '保存顺序'}
            </Button>
          </div>
        }
      />

      <div className="grid max-w-4xl gap-4">
        <Section
          title="会话粘性"
          description="同一会话优先留在上次接单的账号上，保住上游 prompt cache 和多轮工具上下文；账号不可用或失败时仍按上面的顺序换下一个。"
          actions={
            <div className="flex items-center gap-1">
              <Button
                variant={affinityValue === 'session' ? 'secondary' : 'ghost'}
                size="sm"
                onClick={() => saveAffinity('session')}
              >
                会话内保持
              </Button>
              <Button
                variant={affinityValue === 'off' ? 'secondary' : 'ghost'}
                size="sm"
                onClick={() => saveAffinity('off')}
              >
                关闭
              </Button>
            </div>
          }
        >
          <div className="px-4 py-3 text-xs leading-relaxed text-muted-foreground">
            {affinityValue === 'session'
              ? '已开启：会话键取 X-Session-Id / X-Conversation-Id 等请求头；没有时用首条用户消息与 system 提示的哈希。映射 30 分钟未使用自动过期。'
              : '已关闭：每个请求都按路由顺序从头选，不再参考上次接单的账号。'}
          </div>
        </Section>
        {!q.data.configured && !dirty && (
          <div className="flex items-start gap-2 rounded-lg border bg-muted/30 px-4 py-3 text-xs leading-relaxed text-muted-foreground">
            <Shuffle aria-hidden="true" className="mt-0.5 size-3.5 shrink-0" />
            <p>当前还没启用严格顺序。可以直接点「保存顺序」按现在的添加顺序启用，也可以先调整；之后新增的账号会自动排在最后。</p>
          </div>
        )}
        {groups.length === 0 && (
          <Section title="路由顺序"><div className="px-4 py-10 text-center text-sm text-muted-foreground">还没有可排序的账号，请先在「供应商」页添加。</div></Section>
        )}
        {groups.map((group, gi) => (
          <Section
            key={group.key}
            title={supplierLabel(group)}
            description={`${group.accounts.length} 个账号 · 供应商顺序 ${gi + 1}`}
            actions={
              <div className="flex items-center gap-1">
                <Button variant="ghost" size="icon" aria-label="供应商上移" disabled={gi === 0} onClick={() => moveSupplier(gi, gi - 1)}>
                  <ChevronUp />
                </Button>
                <Button variant="ghost" size="icon" aria-label="供应商下移" disabled={gi === groups.length - 1} onClick={() => moveSupplier(gi, gi + 1)}>
                  <ChevronDown />
                </Button>
              </div>
            }
          >
            <div className={cn('divide-y')}>
              {group.accounts.map((account, ai) => (
                <AccountRow
                  key={account.id}
                  account={account}
                  index={ai}
                  total={group.accounts.length}
                  onMove={(from, to) => moveAccount(gi, from, to)}
                />
              ))}
              {group.accounts.length === 0 && <div className="px-4 py-4 text-xs text-muted-foreground">该供应商暂无账号</div>}
            </div>
          </Section>
        ))}
      </div>
    </>
  )
}
