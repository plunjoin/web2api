import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Boxes, Check, Pencil, RotateCcw, Search, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { PageHeader, Section, Toolbar } from '@/components/page'
import { EmptyState, ErrorState, TableSkeleton } from '@/components/states'
import { Pill, StatusDot } from '@/components/status'
import { CopyButton } from '@/components/copy'
import { api } from '@/lib/api'
import { multiplier, relative } from '@/lib/format'
import type { ModelInfo } from '@/lib/types'

interface MultItem {
  model: string
  multiplier: number
  updated_at: number
}

export default function AdminModels() {
  const qc = useQueryClient()
  const models = useQuery({ queryKey: ['admin', 'models'], queryFn: () => api<{ models: ModelInfo[] }>('/admin/api/models') })
  const mults = useQuery({ queryKey: ['admin', 'multipliers'], queryFn: () => api<{ multipliers: MultItem[]; default_multiplier: number; formula: string }>('/admin/api/multipliers') })
  const [search, setSearch] = useState('')
  const [editing, setEditing] = useState<string | null>(null)
  const [value, setValue] = useState('')

  const explicit = useMemo(() => new Map((mults.data?.multipliers || []).map((m) => [m.model, m])), [mults.data])
  const rows = useMemo(() => {
    // 合并：在线模型目录 + 已设置倍率但当前不在目录中的模型。
    const map = new Map<string, { id: string; display?: string; engines: Set<string>; available: boolean }>()
    for (const m of models.data?.models || []) {
      const row = map.get(m.id) || { id: m.id, display: m.display_name, engines: new Set<string>(), available: false }
      if (m.engine) row.engines.add(m.engine)
      row.available ||= m.available
      map.set(m.id, row)
    }
    for (const m of mults.data?.multipliers || []) {
      if (m.model !== '*' && !map.has(m.model)) map.set(m.model, { id: m.model, engines: new Set(), available: false })
    }
    const s = search.trim().toLowerCase()
    return [...map.values()].filter((r) => !s || r.id.toLowerCase().includes(s)).sort((a, b) => a.id.localeCompare(b.id))
  }, [models.data, mults.data, search])

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
      qc.invalidateQueries({ queryKey: ['admin'] })
    } catch (e) {
      toast.error((e as Error).message)
    }
  }
  const reset = async (model: string) => {
    try {
      await api(`/admin/api/multipliers/${encodeURIComponent(model)}`, { method: 'DELETE' })
      toast.success('已恢复为默认倍率')
      qc.invalidateQueries({ queryKey: ['admin'] })
    } catch (e) {
      toast.error((e as Error).message)
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

  return (
    <>
      <PageHeader title="模型与定价" description={'扣费 Token = ⌈总 Token × 模型倍率 × Key 倍率 × 用户倍率⌉。未单独设置的模型使用默认倍率。'} />
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
        <span className="text-xs text-muted-foreground">点击倍率即可编辑，回车保存，Esc 取消</span>
      </Toolbar>
      <Section title={`${rows.length} 个模型`}>
        {models.isLoading || mults.isLoading ? (
          <TableSkeleton rows={8} cols={4} />
        ) : models.isError ? (
          <ErrorState error={models.error} onRetry={() => models.refetch()} />
        ) : !rows.length ? (
          <EmptyState icon={<Boxes />} title={search ? '没有匹配的模型' : '模型目录为空'} description={search ? undefined : '添加上游账号并就绪后，模型会自动出现在这里。'} />
        ) : (
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead>模型 ID</TableHead>
                  <TableHead>引擎</TableHead>
                  <TableHead>状态</TableHead>
                  <TableHead className="w-48 text-right">倍率</TableHead>
                  <TableHead className="w-10" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((r) => {
                  const m = explicit.get(r.id)
                  return (
                    <TableRow key={r.id}>
                      <TableCell>
                        <div className="flex items-center gap-1">
                          <span className="font-mono text-xs">{r.id}</span>
                          <CopyButton value={r.id} label="模型 ID 已复制" className="opacity-50 hover:opacity-100" />
                        </div>
                        {r.display && r.display !== r.id && <div className="text-[11px] text-muted-foreground">{r.display}</div>}
                      </TableCell>
                      <TableCell className="space-x-1">
                        {[...r.engines].map((e) => (
                          <Pill key={e} tone={e === 'a' ? 'primary' : 'muted'}>
                            {e === 'a' ? '网页' : e === 'b' ? 'AI Studio' : e}
                          </Pill>
                        ))}
                        {!r.engines.size && <span className="text-xs text-muted-foreground">不在当前目录</span>}
                      </TableCell>
                      <TableCell>{r.available ? <StatusDot tone="success">可用</StatusDot> : <StatusDot tone="muted">不可用</StatusDot>}</TableCell>
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
                        {m && (
                          <Button variant="ghost" size="icon-sm" onClick={() => reset(r.id)} aria-label="恢复默认" title="恢复默认倍率">
                            <RotateCcw />
                          </Button>
                        )}
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
