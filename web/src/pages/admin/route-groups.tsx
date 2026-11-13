import { Fragment, useEffect, useMemo, useState, type ReactNode } from 'react'
import { Link } from 'react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Boxes, Check, Download, Pencil, RefreshCw, Search, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { PageHeader, Section } from '@/components/page'
import { EmptyState, ErrorState } from '@/components/states'
import { Pill, StatusDot } from '@/components/status'
import { ModelKindFilter, ModelKindPill, modelKindLabel } from '@/components/model-kind'
import { CopyButton } from '@/components/copy'
import { useConfirm } from '@/components/confirm'
import { api } from '@/lib/api'
import { dateTime, multiplier, relative } from '@/lib/format'
import { canonicalEngine, engineLabel, type ModelInfo, type RoutingView } from '@/lib/types'
import { cn } from '@/lib/utils'

interface MultItem {
  model: string
  multiplier: number
  updated_at: number
}

interface GroupMultItem {
  group: string
  multiplier: number
  updated_at: number
}

// 目录快照按「供应商 × 模型」存行；路由组按供应商（引擎）聚合，同名模型跨组共享同一条目。
type ModelRow = {
  key: string // 归一化裸名
  id: string // 展示用的首个原始 ID
  display?: string
  kind?: string
  free?: boolean
  enabled: boolean
  available: boolean
  lastCheckAt?: number
  lastCheckOK?: boolean
  lastCheckDetail?: string
  engines: string[] // 提供该模型的全部路由组（归一化引擎名）
}

type EngineGroup = {
  key: string // 归一化引擎名；'_none' 为只有设置记录、不在目录里的模型
  label: string
  rows: ModelRow[]
  openCount: number
}

type CheckResult = {
  model: string
  ok: boolean
  detail?: string
  enabled: boolean
  setting?: {
    enabled: boolean
    last_check_at?: number
    last_check_ok?: boolean
    last_check_detail?: string
  }
}

type EditTarget = { type: 'default' } | { type: 'group'; key: string } | { type: 'model'; id: string }

// 上游目录可能用 models/x、大写等不同写法列出同一个模型，而设置按裸名存。
const canonicalModelID = (id: string) => id.trim().toLowerCase().replace(/^models\//, '')

// 路由组键：历史名 a/b 归一；第三方 Key 各地址（custom|url）各自成组——
// 用量记录现在也按 custom|<base_url> 归因，供应商级组倍率才能在计费时命中。
const routeGroupKey = (engine: string) => canonicalEngine(engine)

// 组显示名：第三方 Key 只显示主机名，比整段 URL 短、和图里一致。
const groupLabel = (engine: string) => {
  if (engine.startsWith('custom|')) {
    const host = engine.slice('custom|'.length).replace(/^https?:\/\//, '').replace(/^www\./, '').split('/')[0]
    return host || engineLabel(engine)
  }
  return engineLabel(engine)
}

// 供应商头像：模型行用它标出「这个同名模型由哪些供应商提供」。
const ENGINE_AVATARS: Record<string, { bg: string; label: string }> = {
  geminiweb: { bg: 'bg-blue-500', label: 'GW' },
  aistudio: { bg: 'bg-violet-500', label: 'AS' },
  codex: { bg: 'bg-zinc-700', label: 'GPT' },
  antigravity: { bg: 'bg-sky-600', label: 'AG' },
  kiro: { bg: 'bg-orange-500', label: 'KI' },
  workbuddy: { bg: 'bg-emerald-500', label: 'WB' },
  teleagent: { bg: 'bg-cyan-600', label: 'TA' },
  custom: { bg: 'bg-slate-500', label: '3K' },
}

// 第三方 Key 各地址用主机名缩写 + 按地址取色，视觉上和内置引擎区分开。
const PROVIDER_PALETTE = ['bg-emerald-600', 'bg-rose-500', 'bg-amber-500', 'bg-indigo-500', 'bg-teal-600', 'bg-fuchsia-500', 'bg-lime-600', 'bg-red-500']

function providerAvatar(engine: string): { bg: string; label: string } {
  const url = engine.slice('custom|'.length)
  const host = url.replace(/^https?:\/\//, '').replace(/^www\./, '').split('/')[0]
  const letters = host.replace(/[^a-z0-9]/gi, '').slice(0, 2).toUpperCase() || 'K'
  let hash = 0
  for (let i = 0; i < host.length; i++) hash = (hash * 31 + host.charCodeAt(i)) >>> 0
  return { bg: PROVIDER_PALETTE[hash % PROVIDER_PALETTE.length], label: letters }
}

function EngineAvatar({ engine }: { engine: string }) {
  const meta = ENGINE_AVATARS[engine] || (engine.startsWith('custom|') ? providerAvatar(engine) : undefined)
  return (
    <span
      className={cn('grid size-5 shrink-0 place-items-center rounded-md text-[9px] font-semibold text-white', meta?.bg || 'bg-zinc-500')}
      title={engineLabel(engine)}
      aria-label={engineLabel(engine)}
    >
      {meta?.label || engine.slice(0, 2).toUpperCase()}
    </span>
  )
}

export default function AdminRouteGroups() {
  const qc = useQueryClient()
  const models = useQuery({ queryKey: ['admin', 'models'], queryFn: () => api<{ models: ModelInfo[]; fetched_at: number }>('/admin/api/models') })
  const mults = useQuery({ queryKey: ['admin', 'multipliers'], queryFn: () => api<{ multipliers: MultItem[]; groups: GroupMultItem[]; default_multiplier: number; formula: string }>('/admin/api/multipliers') })
  const routing = useQuery({ queryKey: ['admin', 'routing'], queryFn: () => api<RoutingView>('/admin/api/routing') })
  const [search, setSearch] = useState('')
  const [kind, setKind] = useState('all')
  const [status, setStatus] = useState<'all' | 'enabled' | 'disabled'>('all')
  const [selected, setSelected] = useState<string | 'default' | null>(null)
  const [editing, setEditing] = useState<EditTarget | null>(null)
  const [value, setValue] = useState('')
  const [checking, setChecking] = useState<string | null>(null)
  const [toggling, setToggling] = useState(false)
  const [fetching, setFetching] = useState(false)
  const confirm = useConfirm()

  const invalidate = () => qc.invalidateQueries({ queryKey: ['admin', 'models'] })

  // 从每个供应商重新拉取模型目录，成功的供应商整表替换已保存的快照。
  const fetchModels = async () => {
    setFetching(true)
    try {
      const res = await api<{ total: number; engines: { engine: string; models: number; new: number; saved: boolean; error?: string }[] }>(
        '/admin/api/models/refresh',
        { method: 'POST', json: {} },
      )
      const failed = (res.engines || []).filter((e) => !e.saved)
      const saved = (res.engines || []).filter((e) => e.saved)
      const added = saved.reduce((n, e) => n + (e.new || 0), 0)
      if (saved.length) {
        toast.success(
          added
            ? `已获取 ${res.total} 个模型，其中 ${added} 个是新的，默认未开放，在右侧勾选即可开放`
            : `已获取 ${res.total} 个模型，没有新增`,
        )
      }
      if (failed.length) toast.warning(`${failed.length} 个供应商获取失败，保留上次的列表：${failed.map((e) => `${engineLabel(e.engine)}（${e.error || '未知错误'}）`).join('、')}`)
      if (!saved.length && !failed.length) toast.warning('没有可获取模型的供应商，请先添加供应商账号')
      invalidate()
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      setFetching(false)
    }
  }

  // 路由顺序里供应商的位次（custom|地址 归并为 custom 组），决定左侧组的排列。
  const supplierRank = useMemo(() => {
    const rank = new Map<string, number>()
    for (const s of routing.data?.suppliers || []) {
      const key = canonicalEngine(s.engine)
      if (!rank.has(key)) rank.set(key, rank.size)
    }
    return rank
  }, [routing.data])

  // 供应商按路由顺序排：头像簇和「也在」都用这个顺序。
  const rankedEngines = (engines: string[]) =>
    [...engines].sort((a, b) => (supplierRank.get(a) ?? supplierRank.size) - (supplierRank.get(b) ?? supplierRank.size))

  // 目录快照聚合：路由组 = 供应商（引擎），同名模型跨组共享同一条目，
  // 组内勾选开放的是这条共享条目（开关全局生效，所有组看到同一状态）。
  const groups = useMemo<EngineGroup[]>(() => {
    const perEngine = new Map<string, Map<string, ModelRow>>()
    // 同名模型跨组共享：先按裸名收集「这个模型出现在哪些供应商」，
    // 再回填到每行，右侧的「也在 X、Y」才看得到。
    const enginesOf = new Map<string, Set<string>>()
    for (const m of models.data?.models || []) {
      const key = canonicalModelID(m.id)
      const eng = m.engine ? routeGroupKey(m.engine) : '_none'
      if (m.engine) {
        let set = enginesOf.get(key)
        if (!set) {
          set = new Set()
          enginesOf.set(key, set)
        }
        set.add(eng)
      }
      const bucket = perEngine.get(eng) || new Map<string, ModelRow>()
      perEngine.set(eng, bucket)
      const row = bucket.get(key) || {
        key, id: m.id, display: m.display_name, engines: [],
        available: false, enabled: m.enabled !== false,
        kind: m.kind, free: m.free,
        lastCheckAt: m.last_check_at, lastCheckOK: m.last_check_ok, lastCheckDetail: m.last_check_detail,
      }
      row.available ||= m.available
      row.enabled = m.enabled !== false
      if (m.last_check_at) {
        row.lastCheckAt = m.last_check_at
        row.lastCheckOK = m.last_check_ok
        row.lastCheckDetail = m.last_check_detail
      }
      if (m.kind && m.kind !== 'chat') row.kind = m.kind
      else if (!row.kind) row.kind = m.kind
      bucket.set(key, row)
    }
    for (const bucket of perEngine.values()) {
      for (const row of bucket.values()) {
        row.engines = [...(enginesOf.get(row.key) || [])]
      }
    }
    const out: EngineGroup[] = []
    for (const [eng, bucket] of perEngine) {
      const rows = [...bucket.values()].sort((a, b) => a.id.localeCompare(b.id))
      out.push({
        key: eng,
        label: eng === '_none' ? '未归类' : groupLabel(eng),
        rows,
        openCount: rows.filter((r) => r.enabled).length,
      })
    }
    out.sort((a, b) => {
      if (a.key === '_none') return 1
      if (b.key === '_none') return -1
      const ra = supplierRank.get(a.key) ?? supplierRank.size
      const rb = supplierRank.get(b.key) ?? supplierRank.size
      return ra - rb || a.key.localeCompare(b.key)
    })
    return out
  }, [models.data, supplierRank])

  const explicit = useMemo(
    () => new Map((mults.data?.multipliers || []).map((m) => [canonicalModelID(m.model), m])),
    [mults.data],
  )
  const groupMults = useMemo(
    () => new Map((mults.data?.groups || []).map((g) => [g.group, g])),
    [mults.data],
  )
  const def = mults.data?.default_multiplier ?? 1

  const selectedKey = selected ?? 'default'
  const current = selectedKey !== 'default' ? groups.find((g) => g.key === selectedKey) : undefined

  // 筛选只作用于右侧模型列表；左侧组列表始终显示全量（组数很少，不随筛选消失）。
  const visibleRows = useMemo(() => {
    const s = search.trim().toLowerCase()
    return (rows: ModelRow[]) =>
      rows
        .filter((r) => kind === 'all' || r.kind === kind)
        .filter((r) => status === 'all' || (status === 'enabled' ? r.enabled : !r.enabled))
        .filter((r) => !s || r.id.toLowerCase().includes(s) || (r.display || '').toLowerCase().includes(s) || modelKindLabel(r.kind).includes(s))
  }, [search, kind, status])

  // 选中组被删除（供应商目录清空）后回落到默认倍率视图。
  useEffect(() => {
    if (selected && selected !== 'default' && !groups.some((g) => g.key === selected)) setSelected(null)
  }, [groups, selected])

  // 本组模型的生效倍率：模型单独设置 → 组倍率 → 默认。
  const effectiveFor = (row: ModelRow, groupKey: string) => {
    const own = explicit.get(row.key)
    if (own) return { value: own.multiplier, own: true }
    const gm = groupMults.get(groupKey)
    if (gm) return { value: gm.multiplier, own: false }
    return { value: def, own: false }
  }

  const applyCheckToCache = (result: CheckResult) => {
    qc.setQueryData<{ models: ModelInfo[] }>(['admin', 'models'], (prev) => {
      if (!prev) return prev
      const setting = result.setting
      const target = canonicalModelID(result.model)
      return {
        models: prev.models.map((m) =>
          canonicalModelID(m.id) !== target
            ? m
            : {
                ...m,
                // 检查失败时后端只记录检查结果、不改开关；响应不带 model 时
                // 必须保留缓存里原来的开放状态，不能把模型显示成「已下架」。
                enabled: setting?.enabled ?? (result.ok ? result.enabled : m.enabled),
                last_check_at: setting?.last_check_at ?? Math.floor(Date.now() / 1000),
                last_check_ok: setting?.last_check_ok ?? result.ok,
                last_check_detail: setting?.last_check_detail ?? result.detail,
              },
        ),
      }
    })
  }

  const saveValue = async (target: EditTarget) => {
    const v = Number(value)
    if (!(v > 0)) {
      toast.error('倍率必须大于 0')
      return
    }
    try {
      if (target.type === 'group') {
        await api('/admin/api/multipliers/group', { method: 'PUT', json: { group: target.key, multiplier: v } })
        toast.success(`${groupLabel(target.key)} 组倍率已更新`)
      } else if (target.type === 'model') {
        await api('/admin/api/multipliers', { method: 'PUT', json: { model: target.id, multiplier: v } })
        toast.success(`${target.id} 倍率已更新`)
      } else {
        await api('/admin/api/multipliers', { method: 'PUT', json: { model: '*', multiplier: v } })
        toast.success('默认倍率已更新')
      }
      setEditing(null)
      qc.invalidateQueries({ queryKey: ['admin', 'multipliers'] })
    } catch (e) {
      toast.error((e as Error).message)
    }
  }
  const resetValue = async (target: EditTarget) => {
    try {
      if (target.type === 'group') {
        await api(`/admin/api/multipliers/group/${encodeURIComponent(target.key)}`, { method: 'DELETE' })
        toast.success('组倍率已清除')
      } else if (target.type === 'model') {
        await api(`/admin/api/multipliers/${encodeURIComponent(target.id)}`, { method: 'DELETE' })
        toast.success('已恢复组/默认倍率')
      } else {
        return
      }
      qc.invalidateQueries({ queryKey: ['admin', 'multipliers'] })
    } catch (e) {
      toast.error((e as Error).message)
    }
  }

  const setModelsEnabled = async (ids: string[], enabled: boolean, label: string) => {
    if (!ids.length) return
    setToggling(true)
    try {
      await api('/admin/api/models/batch', { method: 'POST', json: { models: ids, enabled } })
      const keys = new Set(ids.map(canonicalModelID))
      qc.setQueryData<{ models: ModelInfo[] }>(['admin', 'models'], (prev) =>
        prev ? { models: prev.models.map((m) => (keys.has(canonicalModelID(m.id)) ? { ...m, enabled } : m)) } : prev,
      )
      toast.success(label)
    } catch (e) {
      toast.error((e as Error).message)
      invalidate()
    } finally {
      setToggling(false)
    }
  }

  const toggleGroup = async (g: EngineGroup, enabled: boolean) => {
    const ids = g.rows.map((r) => r.id)
    if (enabled) {
      await setModelsEnabled(ids, true, `${g.label} 已全部开放（${ids.length} 个模型）`)
      return
    }
    const ok = await confirm({
      title: `下架 ${g.label} 的 ${g.openCount} 个开放模型？`,
      description: '下架后下游 /v1/models 不可见、调用会被拒绝；可随时重新开放整组。',
      confirmText: '全部下架',
      destructive: true,
    })
    if (!ok) return
    await setModelsEnabled(ids, false, `${g.label} 已全部下架`)
  }

  const checkOne = async (id: string, opts?: { quiet?: boolean }) => {
    setChecking(id)
    try {
      const res = await api<{ ok: boolean; detail?: string; model?: CheckResult['setting'] }>(
        '/admin/api/models/check',
        { method: 'POST', json: { model: id, sync_status: true } },
      )
      const result: CheckResult = {
        model: id,
        ok: true,
        detail: res.detail,
        enabled: res.model?.enabled !== false,
        setting: res.model,
      }
      applyCheckToCache(result)
      if (!opts?.quiet) toast.success(res.detail ? `${id} 可用：${res.detail}` : `${id} 检查通过，已开放`)
      return result
    } catch (e) {
      const err = e as Error & { data?: { detail?: string; error?: string; model?: CheckResult['setting'] } }
      const detail = err.data?.error || err.data?.detail || err.message
      const setting = err.data?.model
      applyCheckToCache({ model: id, ok: false, detail, enabled: setting?.enabled === true, setting })
      if (!opts?.quiet) toast.error(`${id} 不可用（未改变开放状态）：${detail}`)
      return { model: id, ok: false, detail, enabled: false, setting } satisfies CheckResult
    } finally {
      setChecking(null)
    }
  }

  const editor = (target: EditTarget, current: number) => (
    <form
      className="flex items-center gap-1"
      onSubmit={(e) => {
        e.preventDefault()
        saveValue(target)
      }}
    >
      <Input autoFocus value={value} onChange={(e) => setValue(e.target.value)} onKeyDown={(e) => e.key === 'Escape' && setEditing(null)} className="h-7 w-24 text-right text-xs tabular-nums" inputMode="decimal" />
      <Button type="submit" size="icon-sm" variant="ghost" aria-label="保存">
        <Check />
      </Button>
      <Button type="button" size="icon-sm" variant="ghost" onClick={() => setEditing(null)} aria-label="取消">
        <X />
      </Button>
      <span className="text-xs text-muted-foreground">当前 {multiplier(current)}</span>
    </form>
  )

  const busy = toggling || checking !== null

  const leftItem = (active: boolean, onClick: () => void, children: ReactNode) => (
    <div
      role="button"
      tabIndex={0}
      onClick={onClick}
      onKeyDown={(e) => e.key === 'Enter' && onClick()}
      className={cn('flex w-full cursor-pointer items-center gap-2 rounded-lg px-2 py-1.5 text-left text-xs focus-visible:outline-2 focus-visible:outline-ring', active ? 'bg-accent' : 'hover:bg-accent/50')}
    >
      {children}
    </div>
  )

  const groupState = (g: EngineGroup): boolean | 'indeterminate' =>
    g.openCount === 0 ? false : g.openCount === g.rows.length ? true : 'indeterminate'

  const statusLine = (r: ModelRow) => {
    let label = <StatusDot tone="muted">未检查</StatusDot>
    if (checking === r.id) label = <StatusDot tone="muted">检查中…</StatusDot>
    else if (!r.enabled) label = <StatusDot tone="danger">已下架</StatusDot>
    else if (r.lastCheckAt && r.lastCheckOK === false) label = <StatusDot tone="danger">检查失败</StatusDot>
    else if (r.lastCheckAt && r.lastCheckOK) label = <StatusDot tone="success">检查通过</StatusDot>
    else if (r.available) label = <StatusDot tone="success">上游可用</StatusDot>
    else label = <StatusDot tone="muted">上游暂不可用</StatusDot>
    return (
      <span className="inline-flex items-center">
        {label}
        {r.lastCheckAt && (
          <span className="ml-1.5 text-[11px] text-muted-foreground" title={r.lastCheckDetail}>
            {relative(r.lastCheckAt)}{r.lastCheckDetail ? ` · ${r.lastCheckDetail}` : ''}
          </span>
        )}
      </span>
    )
  }

  const modelRow = (g: EngineGroup, r: ModelRow) => {
    const eff = effectiveFor(r, g.key)
    const providers = rankedEngines(r.engines)
    const others = providers.filter((e) => e !== g.key)
    return (
      <div key={r.key} className={cn('flex items-start gap-3 border-t px-4 py-2.5', !r.enabled && 'opacity-70')}>
        <div className="pt-0.5">
          <Checkbox
            checked={r.enabled}
            disabled={busy}
            onClick={() => setModelsEnabled([r.id], !r.enabled, r.enabled ? `${r.id} 已下架` : `${r.id} 已开放`)}
            aria-label={r.enabled ? `下架 ${r.id}` : `开放 ${r.id}`}
            title={r.enabled ? '取消勾选即下架（对所有组生效）' : '勾选即开放'}
          />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-1.5">
            <span className="font-mono text-sm">{r.id}</span>
            {r.free && <Pill tone="success">免费</Pill>}
            <ModelKindPill kind={r.kind} />
            <CopyButton value={r.id} label="模型 ID 已复制" className="opacity-40 hover:opacity-100" />
          </div>
          {r.display && r.display !== r.id && <div className="mt-0.5 text-[11px] text-muted-foreground">{r.display}</div>}
          <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
            {statusLine(r)}
            {/* 同名模型路由组：头像 = 提供这个模型的全部供应商（含当前组），同名模型共用一条目一起接单 */}
            <span
              className="inline-flex items-center gap-1"
              title={`提供该模型的供应商：${providers.map(engineLabel).join('、')}${others.length ? `（也在 ${others.map(engineLabel).join('、')}）` : ''}`}
            >
              {providers.map((e) => (
                <EngineAvatar key={e} engine={e} />
              ))}
              {providers.length > 1 && <span>同名 · {providers.length} 个供应商</span>}
            </span>
          </div>
        </div>
        <div className="shrink-0 pt-0.5 text-right">
          {editing?.type === 'model' && editing.id === r.id ? (
            editor({ type: 'model', id: r.id }, eff.value)
          ) : (
            <button
              className="inline-flex items-center gap-1.5 rounded px-1.5 py-0.5 tabular-nums hover:bg-accent"
              onClick={() => { setEditing({ type: 'model', id: r.id }); setValue(String(eff.value)) }}
              title={eff.own ? '模型单独倍率，点击修改' : '继承组/默认倍率，点击设置模型倍率'}
            >
              <span className={cn('text-sm', eff.own ? 'font-semibold' : 'text-muted-foreground')}>
                {multiplier(eff.value)}
                {!eff.own && ' 继承'}
              </span>
              <Pencil className="size-3 opacity-40" />
            </button>
          )}
          {explicit.has(r.key) && editing?.type !== 'model' && (
            <Button variant="ghost" size="sm" className="ml-1 h-6 px-1.5 text-xs" onClick={() => resetValue({ type: 'model', id: r.id })} title="清除模型倍率，恢复组/默认">
              恢复
            </Button>
          )}
        </div>
        <Button
          variant="ghost"
          size="icon-sm"
          className="shrink-0"
          disabled={busy}
          onClick={() => checkOne(r.id)}
          aria-label="检查模型"
          title="探测该模型（通过后自动开放，失败只记录）"
        >
          <RefreshCw className={checking === r.id ? 'animate-spin' : undefined} />
        </Button>
      </div>
    )
  }

  const groupMultiplierBlock = (g: EngineGroup) => {
    const gm = groupMults.get(g.key)
    const value = gm?.multiplier ?? def
    return (
      <div className="flex flex-wrap items-center justify-between gap-3 border-b bg-muted/30 px-4 py-2.5">
        <div className="min-w-0">
          <p className="text-xs font-medium">组倍率</p>
          <p className="text-[11px] text-muted-foreground">作用于本组内未单独设置倍率的模型；模型倍率优先于组倍率，组倍率优先于默认倍率。</p>
        </div>
        {editing?.type === 'group' && editing.key === g.key ? (
          editor({ type: 'group', key: g.key }, value)
        ) : (
          <div className="flex items-center gap-1">
            <button
              className="inline-flex items-center gap-1.5 rounded px-1.5 py-0.5 tabular-nums hover:bg-accent"
              onClick={() => { setEditing({ type: 'group', key: g.key }); setValue(String(value)) }}
              title={gm ? `更新于 ${relative(gm.updated_at)}` : '使用默认倍率'}
            >
              <span className={cn('text-base', gm ? 'font-semibold' : 'text-muted-foreground')}>
                {multiplier(value)}
                {!gm && ' 默认'}
              </span>
              <Pencil className="size-3 opacity-40" />
            </button>
            {gm && (
              <Button variant="ghost" size="sm" className="h-6 px-1.5 text-xs" onClick={() => resetValue({ type: 'group', key: g.key })}>
                恢复
              </Button>
            )}
          </div>
        )}
      </div>
    )
  }

  const currentRows = useMemo(() => (current ? visibleRows(current.rows) : []), [current, visibleRows])

  return (
    <>
      <PageHeader
        title="路由"
        description="供应商加入并「获取模型」后自动成一个路由组；同名模型跨组自动识别为同一条目，开放状态与倍率全局共享。倍率取值：模型 → 组 → 默认。"
        actions={
          <Button onClick={fetchModels} disabled={fetching}>
            <Download className={fetching ? 'animate-pulse' : undefined} />
            {fetching ? '各供应商获取中…' : '获取模型'}
          </Button>
        }
      />
      <div className="grid items-start gap-4 lg:grid-cols-[17rem_minmax(0,1fr)]">
        <aside className="overflow-hidden rounded-xl border bg-card">
          <div className="border-b p-2">
            <div className="relative">
              <Search className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
              <Input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="搜索模型" className="h-8 pl-8 text-xs" />
            </div>
            <ModelKindFilter value={kind} onChange={setKind} />
            <div className="mt-2 flex items-center gap-1">
              {([
                ['all', '全部'],
                ['enabled', '已开放'],
                ['disabled', '已下架'],
              ] as const).map(([k, label]) => (
                <Button key={k} size="sm" variant={status === k ? 'secondary' : 'ghost'} className="h-6 px-2 text-xs" onClick={() => setStatus(k)}>
                  {label}
                </Button>
              ))}
            </div>
          </div>
          <div className="max-h-[calc(100dvh-22rem)] overflow-y-auto p-1.5">
            {leftItem(selectedKey === 'default', () => setSelected('default'), (
              <>
                <span className="flex-1 font-medium">默认倍率</span>
                <span className="tabular-nums text-muted-foreground">{multiplier(def)}</span>
              </>
            ))}
            {groups.map((g) => (
              <Fragment key={g.key}>
                {leftItem(selectedKey === g.key, () => setSelected(g.key), (
                  <>
                    <Checkbox
                      checked={groupState(g)}
                      disabled={busy || !g.rows.length}
                      onClick={(e) => {
                        e.stopPropagation()
                        toggleGroup(g, g.openCount < g.rows.length)
                      }}
                      aria-label={g.openCount < g.rows.length ? `开放 ${g.label} 全部模型` : `下架 ${g.label} 全部模型`}
                      title="勾选=整组开放，取消=整组下架"
                    />
                    {g.key !== '_none' && <EngineAvatar engine={g.key} />}
                    <span className="min-w-0 flex-1 truncate" title={g.label}>{g.label}</span>
                    <span className={cn('shrink-0 tabular-nums', g.openCount === 0 ? 'text-muted-foreground' : 'text-foreground')}>
                      {g.openCount}/{g.rows.length}
                    </span>
                  </>
                ))}
              </Fragment>
            ))}
            {!groups.length && (
              <p className="px-2 py-6 text-center text-xs text-muted-foreground">还没有路由组</p>
            )}
          </div>
        </aside>

        <div className="space-y-4">
          {models.isLoading || mults.isLoading ? (
            <Section title="路由组"><div className="p-4 text-sm text-muted-foreground">加载中…</div></Section>
          ) : models.isError ? (
            <Section title="路由组"><ErrorState error={models.error} onRetry={() => models.refetch()} /></Section>
          ) : !groups.length ? (
            <Section title="路由组">
              <EmptyState
                icon={<Boxes />}
                title="还没有路由组"
                description="点右上角「获取模型」，向每个供应商拉取最新列表；供应商有模型后自动出现在左侧。"
                action={
                  <Button size="sm" onClick={fetchModels} disabled={fetching}>
                    <Download className={fetching ? 'animate-pulse' : undefined} /> 获取模型
                  </Button>
                }
              />
            </Section>
          ) : selectedKey === 'default' ? (
            <Section title="默认倍率" description="所有没有单独设置倍率、所在组也没有组倍率的模型都按它计费。扣费 Token = ⌈总 Token × 模型倍率 × Key 倍率 × 用户倍率⌉。">
              <div className="flex items-center justify-between gap-3 px-4 py-3">
                <div className="text-2xl font-semibold tabular-nums">{multiplier(def)}</div>
                {editing?.type === 'default' ? (
                  editor({ type: 'default' }, def)
                ) : (
                  <Button variant="outline" size="sm" onClick={() => { setEditing({ type: 'default' }); setValue(String(def)) }}>
                    <Pencil /> 修改
                  </Button>
                )}
              </div>
            </Section>
          ) : current ? (
            <Section
              title={current.label}
              description={`${current.rows.length} 个模型 · 开放 ${current.openCount} · 上次获取 ${models.data?.fetched_at ? dateTime(models.data.fetched_at) : '从未'}${current.key === '_none' ? ' · 这些模型不在任何供应商目录里，不会被路由到' : ''}`}
            >
              {current.key !== '_none' && groupMultiplierBlock(current)}
              <div>
                {currentRows.map((r) => modelRow(current, r))}
                {!currentRows.length && (
                  <p className="px-4 py-6 text-center text-xs text-muted-foreground">没有匹配当前筛选的模型</p>
                )}
              </div>
              <div className="border-t px-4 py-2 text-[11px] text-muted-foreground">
                {currentRows.length === current.rows.length
                  ? `共 ${current.rows.length} 个模型`
                  : `筛选出 ${currentRows.length}/${current.rows.length} 个模型`} · 开放状态与倍率是全局设置，同名模型在所有组里保持一致 · 供应商接单顺序在{' '}
                <Link to="/admin/routing" className="underline underline-offset-2">路由顺序</Link>
                里调整
              </div>
            </Section>
          ) : (
            <Section title="路由组">
              <EmptyState icon={<Boxes />} title="选择一个路由组" description="在左侧选择供应商查看它的模型、设置组倍率与上下架；或选择「默认倍率」。" />
            </Section>
          )}
        </div>
      </div>
    </>
  )
}
