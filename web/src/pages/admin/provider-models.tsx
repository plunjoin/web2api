import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { CheckCircle2, Download, Plus, RefreshCw, Search, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { EmptyState, ErrorState, TableSkeleton } from '@/components/states'
import { StatusDot } from '@/components/status'
import { ModelKindPill } from '@/components/model-kind'
import { api } from '@/lib/api'
import { engineLabel, type ModelInfo } from '@/lib/types'

type CheckResult = { model: string; ok: boolean; detail?: string; enabled: boolean }

export interface ProviderTarget {
  engine: string
  /** 第三方 Key 的供应商地址（归一化 base_url）。内置引擎不传。 */
  provider?: string
}

/** 某个供应商的模型：只向它拉取、勾选开放哪些、逐个测试是否可用。 */
export function ProviderModelsDialog({
  target,
  onOpenChange,
  onAddAccount,
}: {
  target: ProviderTarget | null
  onOpenChange: (open: boolean) => void
  onAddAccount?: (engine: string) => void
}) {
  const engine = target?.engine ?? null
  // 第三方 Key 的快照按上游地址分开存，键是 "custom|<base_url>"。
  const catalogKey = target?.provider && target.provider !== 'custom|unknown' ? target.provider : engine
  const qc = useQueryClient()
  const models = useQuery({
    queryKey: ['admin', 'models'],
    queryFn: () => api<{ models: ModelInfo[]; fetched_at: number }>('/admin/api/models'),
    enabled: !!engine,
  })
  const [search, setSearch] = useState('')
  const [fetching, setFetching] = useState(false)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [applying, setApplying] = useState(false)
  const [testing, setTesting] = useState<string | null>(null)
  const [testingAll, setTestingAll] = useState(false)
  const [progress, setProgress] = useState('')
  const [results, setResults] = useState<Record<string, CheckResult>>({})

  const rows = useMemo(() => {
    const seen = new Set<string>()
    const out: ModelInfo[] = []
    for (const m of models.data?.models || []) {
      if (m.engine !== catalogKey || seen.has(m.id)) continue
      seen.add(m.id)
      out.push(m)
    }
    const s = search.trim().toLowerCase()
    return out
      .filter((m) => !s || m.id.toLowerCase().includes(s) || (m.display_name || '').toLowerCase().includes(s))
      .sort((a, b) => Number(b.enabled !== false) - Number(a.enabled !== false) || a.id.localeCompare(b.id))
  }, [models.data, catalogKey, search])

  const refresh = async () => {
    if (!engine) return
    setFetching(true)
    try {
      const res = await api<{ models: number; new: number }>(`/admin/api/engines/${encodeURIComponent(engine)}/models/refresh`, {
        method: 'POST',
        // 第三方 Key 只向这一个上游地址拉取，不影响其他供应商。
        json: catalogKey !== engine ? { provider: catalogKey } : {},
      })
      toast.success(res.new ? `已获取 ${res.models} 个模型，其中 ${res.new} 个是新的，默认未开放` : `已获取 ${res.models} 个模型，没有新增`)
      setResults({})
      await qc.invalidateQueries({ queryKey: ['admin', 'models'] })
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      setFetching(false)
    }
  }

  const apply = async (ids: string[], enabled: boolean) => {
    if (!ids.length) return
    setApplying(true)
    try {
      await api('/admin/api/models/batch', { method: 'POST', json: { models: ids, enabled } })
      toast.success(enabled ? `已开放 ${ids.length} 个模型` : `已下架 ${ids.length} 个模型`)
      setSelected(new Set())
      await qc.invalidateQueries({ queryKey: ['admin', 'models'] })
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      setApplying(false)
    }
  }

  const testOne = async (id: string): Promise<boolean> => {
    setTesting(id)
    try {
      const res = await api<{ detail?: string; model?: { enabled?: boolean } }>('/admin/api/models/check', {
        method: 'POST',
        json: { model: id, sync_status: false },
      })
      setResults((prev) => ({ ...prev, [id]: { model: id, ok: true, detail: res.detail, enabled: res.model?.enabled !== false } }))
      return true
    } catch (e) {
      const err = e as Error & { data?: { error?: string; detail?: string } }
      setResults((prev) => ({ ...prev, [id]: { model: id, ok: false, detail: err.data?.error || err.data?.detail || err.message, enabled: false } }))
      return false
    } finally {
      setTesting(null)
    }
  }

  const testMany = async (ids: string[]) => {
    if (!ids.length) return
    setTestingAll(true)
    let passed = 0
    try {
      for (let i = 0; i < ids.length; i++) {
        setProgress(`${i + 1}/${ids.length}`)
        if (await testOne(ids[i])) passed++
      }
      toast.message(`测试完成：通过 ${passed}，失败 ${ids.length - passed}`)
    } finally {
      setTestingAll(false)
      setProgress('')
    }
  }

  const enabledCount = rows.filter((m) => m.enabled !== false).length

  return (
    <Dialog open={!!engine} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85vh] flex-col sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle className="text-[15px]">
            {catalogKey && catalogKey !== engine ? catalogKey.replace(/^custom\|/, '') : engine ? engineLabel(engine) : ''} 的模型
          </DialogTitle>
          <DialogDescription className="text-[13px]">
            只向这个供应商拉取。新出现的模型默认未开放，打开开关或勾选后批量开放才对下游可见；测试只探测、不改变开放状态。
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-wrap items-center gap-2">
          <div className="relative">
            <Search className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="搜索模型" className="h-7 w-52 pl-8 text-xs" />
          </div>
          <span className="text-[11px] text-muted-foreground tabular-nums">
            {rows.length} 个 · 已开放 {enabledCount}
          </span>
          <div className="ml-auto flex items-center gap-1.5">
            {selected.size > 0 && (
              <div className="flex items-center gap-0.5 rounded-md border bg-muted/40 py-0.5 pl-2 pr-0.5">
                <span className="px-1 text-xs text-muted-foreground tabular-nums">已选 {selected.size}</span>
                <Button size="sm" variant="ghost" className="h-6 px-2 text-xs" disabled={applying} onClick={() => apply([...selected], true)}>
                  开放
                </Button>
                <Button size="sm" variant="ghost" className="h-6 px-2 text-xs" disabled={applying} onClick={() => apply([...selected], false)}>
                  下架
                </Button>
                <Button size="sm" variant="ghost" className="h-6 px-2 text-xs" disabled={testingAll} onClick={() => testMany([...selected])}>
                  测试
                </Button>
                <Button size="sm" variant="ghost" className="h-6 px-1.5 text-xs" onClick={() => setSelected(new Set())} aria-label="清除选择">
                  <X className="size-3" />
                </Button>
              </div>
            )}
            {progress && <span className="text-xs text-muted-foreground tabular-nums">{progress}</span>}
            <Button size="sm" variant="outline" disabled={testingAll || !rows.length} onClick={() => testMany(rows.map((m) => m.id))}>
              <RefreshCw className={testingAll ? 'animate-spin' : undefined} />
              {testingAll ? '测试中…' : '测试全部'}
            </Button>
            <Button size="sm" onClick={refresh} disabled={fetching}>
              <Download className={fetching ? 'animate-pulse' : undefined} />
              {fetching ? '获取中…' : '获取模型'}
            </Button>
            {onAddAccount && engine && (
              <Button size="sm" variant="outline" onClick={() => onAddAccount(engine)}>
                <Plus /> 添加账号
              </Button>
            )}
          </div>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto rounded-md border">
          {models.isLoading ? (
            <TableSkeleton rows={6} cols={3} />
          ) : models.isError ? (
            <ErrorState error={models.error} onRetry={() => models.refetch()} />
          ) : !rows.length ? (
            <EmptyState
              icon={<Download />}
              title={search ? '没有匹配的模型' : '还没有这个供应商的模型'}
              description={search ? undefined : '点右上角「获取模型」，只向这个供应商拉取。'}
            />
          ) : (
            <ul className="divide-y">
              <li className="flex items-center gap-3 bg-muted/30 px-3 py-1.5">
                <Checkbox
                  checked={rows.every((m) => selected.has(m.id)) ? true : rows.some((m) => selected.has(m.id)) ? 'indeterminate' : false}
                  onCheckedChange={(v) => setSelected(v === true ? new Set(rows.map((m) => m.id)) : new Set())}
                  aria-label="全选"
                />
                <span className="text-[11px] text-muted-foreground">全选当前列表</span>
              </li>
              {rows.map((m) => {
                const result = results[m.id]
                const busy = testing === m.id
                return (
                  <li key={m.id} className="flex items-center gap-3 px-3 py-2">
                    <Checkbox
                      checked={selected.has(m.id)}
                      onCheckedChange={(v) =>
                        setSelected((prev) => {
                          const next = new Set(prev)
                          if (v) next.add(m.id)
                          else next.delete(m.id)
                          return next
                        })
                      }
                      aria-label={`选择 ${m.id}`}
                    />
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-1.5">
                        <span className="truncate font-mono text-xs">{m.id}</span>
                        <ModelKindPill kind={m.kind} />
                      </div>
                      {result ? (
                        <div className="mt-0.5 truncate text-[11px] text-muted-foreground" title={result.detail}>
                          {result.ok ? <StatusDot tone="success">可用</StatusDot> : <StatusDot tone="danger">不可用</StatusDot>}
                          {result.detail ? ` · ${result.detail}` : ''}
                        </div>
                      ) : m.last_check_detail ? (
                        <div className="mt-0.5 truncate text-[11px] text-muted-foreground">{m.last_check_detail}</div>
                      ) : null}
                    </div>
                    <Button size="sm" variant="ghost" className="h-7 px-2 text-xs" disabled={busy} onClick={() => testOne(m.id)}>
                      {busy ? <RefreshCw className="animate-spin" /> : <CheckCircle2 />}
                      测试
                    </Button>
                    <Switch
                      checked={m.enabled !== false}
                      disabled={applying}
                      onCheckedChange={(v) => apply([m.id], v)}
                      aria-label={m.enabled !== false ? '下架' : '开放'}
                    />
                  </li>
                )
              })}
            </ul>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
