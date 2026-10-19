import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Boxes, Check, Pencil, RefreshCw, RotateCcw, Search, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { PageHeader, Section, Toolbar } from '@/components/page'
import { EmptyState, ErrorState, TableSkeleton } from '@/components/states'
import { Pill, StatusDot } from '@/components/status'
import { ModelKindFilter, ModelKindPill, modelKindLabel } from '@/components/model-kind'
import { CopyButton } from '@/components/copy'
import { api } from '@/lib/api'
import { multiplier, relative } from '@/lib/format'
import type { ModelInfo } from '@/lib/types'

interface MultItem {
  model: string
  multiplier: number
  updated_at: number
}

type Row = {
  id: string
  display?: string
  engines: Set<string>
  available: boolean
  enabled: boolean
  kind?: string
  lastCheckAt?: number
  lastCheckOK?: boolean
  lastCheckDetail?: string
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

export default function AdminModels() {
  const qc = useQueryClient()
  const models = useQuery({ queryKey: ['admin', 'models'], queryFn: () => api<{ models: ModelInfo[] }>('/admin/api/models') })
  const mults = useQuery({ queryKey: ['admin', 'multipliers'], queryFn: () => api<{ multipliers: MultItem[]; default_multiplier: number; formula: string }>('/admin/api/multipliers') })
  const [search, setSearch] = useState('')
  const [kind, setKind] = useState('all')
  const [status, setStatus] = useState<'all' | 'enabled' | 'disabled'>('all')
  const [editing, setEditing] = useState<string | null>(null)
  const [value, setValue] = useState('')
  const [checking, setChecking] = useState<string | null>(null)
  const [batching, setBatching] = useState(false)
  const [batchProgress, setBatchProgress] = useState('')

  const invalidate = () => qc.invalidateQueries({ queryKey: ['admin', 'models'] })
  const explicit = useMemo(() => new Map((mults.data?.multipliers || []).map((m) => [m.model, m])), [mults.data])
  const rows = useMemo(() => {
    const map = new Map<string, Row>()
    for (const m of models.data?.models || []) {
      const row = map.get(m.id) || {
        id: m.id,
        display: m.display_name,
        engines: new Set<string>(),
        available: false,
        enabled: m.enabled !== false,
        kind: m.kind,
        lastCheckAt: m.last_check_at,
        lastCheckOK: m.last_check_ok,
        lastCheckDetail: m.last_check_detail,
      }
      if (m.engine) row.engines.add(m.engine)
      row.available ||= m.available
      row.enabled = m.enabled !== false
      if (m.last_check_at) {
        row.lastCheckAt = m.last_check_at
        row.lastCheckOK = m.last_check_ok
        row.lastCheckDetail = m.last_check_detail
      }
      if (m.kind && m.kind !== 'chat') row.kind = m.kind
      else if (!row.kind) row.kind = m.kind
      map.set(m.id, row)
    }
    for (const m of mults.data?.multipliers || []) {
      if (m.model !== '*' && !map.has(m.model)) map.set(m.model, { id: m.model, engines: new Set(), available: false, enabled: true })
    }
    const s = search.trim().toLowerCase()
    return [...map.values()]
      .filter((r) => kind === 'all' || r.kind === kind)
      .filter((r) => status === 'all' || (status === 'enabled' ? r.enabled : !r.enabled))
      .filter((r) => !s || r.id.toLowerCase().includes(s) || (r.display || '').toLowerCase().includes(s) || modelKindLabel(r.kind).includes(s))
      .sort((a, b) => a.id.localeCompare(b.id))
  }, [models.data, mults.data, search, kind, status])

  const applyCheckToCache = (result: CheckResult) => {
    qc.setQueryData<{ models: ModelInfo[] }>(['admin', 'models'], (prev) => {
      if (!prev) return prev
      const setting = result.setting
      return {
        models: prev.models.map((m) =>
          m.id !== result.model
            ? m
            : {
                ...m,
                enabled: setting?.enabled ?? result.enabled,
                last_check_at: setting?.last_check_at ?? Math.floor(Date.now() / 1000),
                last_check_ok: setting?.last_check_ok ?? result.ok,
                last_check_detail: setting?.last_check_detail ?? result.detail,
              },
        ),
      }
    })
  }

  const save = async (model: string) => {
    const v = Number(value)
    if (!(v > 0)) {
      toast.error('倍率必须大于 0')
      return
    }
    try {
      await api('/admin/api/multipliers', { method: 'PUT', json: { model, multiplier: v } })
      toast.success(model === '*' ? '默认倍率已更新' : `${model} 倍率已更新`)
      setEditing(null)
      qc.invalidateQueries({ queryKey: ['admin', 'multipliers'] })
    } catch (e) {
      toast.error((e as Error).message)
    }
  }
  const reset = async (model: string) => {
    try {
      await api(`/admin/api/multipliers/${encodeURIComponent(model)}`, { method: 'DELETE' })
      toast.success('已恢复为默认倍率')
      qc.invalidateQueries({ queryKey: ['admin', 'multipliers'] })
    } catch (e) {
      toast.error((e as Error).message)
    }
  }
  const setEnabled = async (id: string, enabled: boolean) => {
    // 先乐观更新，避免开关“点了没反应”
    qc.setQueryData<{ models: ModelInfo[] }>(['admin', 'models'], (prev) => {
      if (!prev) return prev
      return { models: prev.models.map((m) => (m.id === id ? { ...m, enabled } : m)) }
    })
    try {
      await api(`/admin/api/models/${encodeURIComponent(id)}`, { method: 'PATCH', json: { enabled } })
      toast.success(enabled ? `${id} 已开放` : `${id} 已下架，下游无法获取`)
    } catch (e) {
      toast.error((e as Error).message)
      invalidate()
    }
  }
  const checkOne = async (id: string, opts?: { quiet?: boolean }) => {
    setChecking(id)
    try {
      const res = await api<{ ok: boolean; detail?: string; model?: CheckResult['setting']; results?: CheckResult[] }>(
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
      const err = e as Error & { data?: { detail?: string; error?: string; model?: CheckResult['setting']; results?: CheckResult[] } }
      const detail = err.data?.error || err.data?.detail || err.message
      const setting = err.data?.model
      applyCheckToCache({
        model: id,
        ok: false,
        detail,
        enabled: setting?.enabled === true ? true : false,
        setting,
      })
      if (!opts?.quiet) toast.error(`${id} 不可用，已下架：${detail}`)
      return { model: id, ok: false, detail, enabled: false, setting } satisfies CheckResult
    } finally {
      setChecking(null)
    }
  }
  const checkBatch = async () => {
    // 对话 / 生图 / 向量可真实探测；视频/音乐/语音走目录检查。
    const targets = rows
    if (!targets.length) {
      toast.error('当前列表没有可检查的模型')
      return
    }
    setBatching(true)
    let passed = 0
    let failed = 0
    try {
      for (let i = 0; i < targets.length; i++) {
        const id = targets[i].id
        setBatchProgress(`${i + 1}/${targets.length} ${id}`)
        const result = await checkOne(id, { quiet: true })
        if (result.ok) passed++
        else failed++
      }
      if (failed === 0) toast.success(`批量检查完成：${passed} 个全部通过并已开放`)
      else toast.warning(`批量检查完成：通过 ${passed}，失败并下架 ${failed}`)
    } finally {
      setBatching(false)
      setBatchProgress('')
      setChecking(null)
      invalidate()
    }
  }

  const def = mults.data?.default_multiplier ?? 1
  const editor = (model: string) => (
    <form
      className="flex items-center justify-end gap-1"
      onSubmit={(e) => {
        e.preventDefault()
        save(model)
      }}
    >
      <Input autoFocus value={value} onChange={(e) => setValue(e.target.value)} onKeyDown={(e) => e.key === 'Escape' && setEditing(null)} className="h-7 w-20 text-right text-xs tabular-nums" inputMode="decimal" />
      <Button type="submit" size="icon-sm" variant="ghost" aria-label="保存">
        <Check />
      </Button>
      <Button type="button" size="icon-sm" variant="ghost" onClick={() => setEditing(null)} aria-label="取消">
        <X />
      </Button>
    </form>
  )

  const statusCell = (r: Row) => {
    const busy = checking === r.id
    let label = <StatusDot tone="muted">未检查</StatusDot>
    if (busy) label = <StatusDot tone="muted">检查中…</StatusDot>
    else if (!r.enabled) label = <StatusDot tone="danger">已下架</StatusDot>
    else if (r.lastCheckAt && r.lastCheckOK === false) label = <StatusDot tone="danger">检查失败</StatusDot>
    else if (r.lastCheckAt && r.lastCheckOK) label = <StatusDot tone="success">检查通过</StatusDot>
    else if (r.available) label = <StatusDot tone="success">上游可用</StatusDot>
    else label = <StatusDot tone="muted">上游暂不可用</StatusDot>

    const detailLine =
      r.enabled && r.lastCheckAt
        ? `${relative(r.lastCheckAt)}${r.lastCheckDetail ? ` · ${r.lastCheckDetail}` : ''}`
        : ''

    return (
      <div className="flex w-[16rem] items-center gap-3">
        <div className="min-w-0 flex-1 overflow-hidden">
          {label}
          {!r.enabled && <div className="mt-0.5 text-[11px] text-muted-foreground">下游不可见</div>}
          {detailLine ? (
            <div className="mt-0.5 truncate text-[11px] text-muted-foreground" title={detailLine}>
              {detailLine}
            </div>
          ) : null}
        </div>
        <Switch
          checked={r.enabled}
          disabled={busy || batching}
          onCheckedChange={(v) => setEnabled(r.id, v)}
          aria-label={r.enabled ? '下架模型' : '开放模型'}
          title={r.enabled ? '关闭后下游无法获取' : '开启后下游可见'}
        />
      </div>
    )
  }

  const busy = batching || checking !== null

  return (
    <>
      <PageHeader
        title="模型与定价"
        description="扣费 Token = ⌈总 Token × 模型倍率 × Key 倍率 × 用户倍率⌉。检查模型会按结果自动上下架；也可用开关手动控制。下架后下游 /v1/models 不可见。"
      />
      <Section title="默认倍率" description="适用于所有没有单独设置倍率的模型（键为 *）" className="mb-4">
        <div className="flex items-center justify-between gap-3 px-4 py-3">
          <div className="text-2xl font-semibold tabular-nums">{multiplier(def)}</div>
          {editing === '*' ? (
            editor('*')
          ) : (
            <Button variant="outline" size="sm" onClick={() => { setEditing('*'); setValue(String(def)) }}>
              <Pencil /> 修改
            </Button>
          )}
        </div>
      </Section>
      <Toolbar className="mb-3">
        <div className="relative">
          <Search className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="搜索模型" className="h-7 w-56 pl-8 text-xs" />
        </div>
        <ModelKindFilter value={kind} onChange={setKind} />
        <div className="flex items-center gap-1">
          {([
            ['all', '全部'],
            ['enabled', '已开放'],
            ['disabled', '已下架'],
          ] as const).map(([k, label]) => (
            <Button key={k} size="sm" variant={status === k ? 'secondary' : 'ghost'} className="h-7 px-2 text-xs" onClick={() => setStatus(k)}>
              {label}
            </Button>
          ))}
        </div>
        <div className="ml-auto flex items-center gap-2">
          {batchProgress && <span className="text-xs text-muted-foreground tabular-nums">{batchProgress}</span>}
          <Button size="sm" variant="outline" disabled={busy || !rows.length} onClick={checkBatch}>
            <RefreshCw className={batching ? 'animate-spin' : undefined} />
            {batching ? '检查中…' : '批量检查'}
          </Button>
        </div>
      </Toolbar>
      <Section title={`${rows.length} 个模型`}>
        {models.isLoading || mults.isLoading ? (
          <TableSkeleton rows={8} cols={6} />
        ) : models.isError ? (
          <ErrorState error={models.error} onRetry={() => models.refetch()} />
        ) : !rows.length ? (
          <EmptyState icon={<Boxes />} title={search || kind !== 'all' || status !== 'all' ? '没有匹配的模型' : '模型目录为空'} description={search || kind !== 'all' || status !== 'all' ? undefined : '添加上游账号并就绪后，模型会自动出现在这里。'} />
        ) : (
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead>模型 ID</TableHead>
                  <TableHead>用途</TableHead>
                  <TableHead>引擎</TableHead>
                  <TableHead className="w-[16rem]">状态 / 开放</TableHead>
                  <TableHead className="w-48 text-right">倍率</TableHead>
                  <TableHead className="w-24 text-right">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((r) => {
                  const m = explicit.get(r.id)
                  const rowBusy = checking === r.id
                  return (
                    <TableRow key={r.id} className={!r.enabled ? 'opacity-70' : undefined}>
                      <TableCell>
                        <div className="flex items-center gap-1">
                          <span className="font-mono text-xs">{r.id}</span>
                          <CopyButton value={r.id} label="模型 ID 已复制" className="opacity-50 hover:opacity-100" />
                        </div>
                        {r.display && r.display !== r.id && <div className="text-[11px] text-muted-foreground">{r.display}</div>}
                      </TableCell>
                      <TableCell>
                        <ModelKindPill kind={r.kind} />
                      </TableCell>
                      <TableCell className="space-x-1">
                        {[...r.engines].map((e) => (
                          <Pill key={e} tone={e === 'a' ? 'primary' : 'muted'}>
                            {e === 'a' ? '网页' : e === 'b' ? 'AI Studio' : e}
                          </Pill>
                        ))}
                        {!r.engines.size && <span className="text-xs text-muted-foreground">不在当前目录</span>}
                      </TableCell>
                      <TableCell className="max-w-[16rem]">{statusCell(r)}</TableCell>
                      <TableCell className="text-right">
                        {editing === r.id ? (
                          editor(r.id)
                        ) : (
                          <button
                            className="inline-flex items-center gap-1.5 rounded px-1.5 py-0.5 tabular-nums hover:bg-accent"
                            onClick={() => { setEditing(r.id); setValue(String(m?.multiplier ?? def)) }}
                            title={m ? `更新于 ${relative(m.updated_at)}` : '使用默认倍率'}
                          >
                            {m ? <span className="font-medium">{multiplier(m.multiplier)}</span> : <span className="text-muted-foreground">{multiplier(def)} 默认</span>}
                            <Pencil className="size-3 opacity-40" />
                          </button>
                        )}
                      </TableCell>
                      <TableCell>
                        <div className="flex items-center justify-end gap-0.5">
                          {m && (
                            <Button variant="ghost" size="icon-sm" onClick={() => reset(r.id)} aria-label="恢复默认" title="恢复默认倍率">
                              <RotateCcw />
                            </Button>
                          )}
                          <Button
                            variant="ghost"
                            size="icon-sm"
                            disabled={busy}
                            onClick={() => checkOne(r.id)}
                            aria-label="检查模型"
                            title="检查并同步上下架"
                          >
                            <RefreshCw className={rowBusy ? 'animate-spin' : undefined} />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          </div>
        )}
      </Section>
    </>
  )
}
