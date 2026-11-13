import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Check, Pencil, Plus, Trash2, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { PageHeader, Section } from '@/components/page'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { EmptyState, ErrorState, TableSkeleton } from '@/components/states'
import { Field } from '@/components/fields'
import { useConfirm } from '@/components/confirm'
import { api } from '@/lib/api'
import { dateTime } from '@/lib/format'
import type { ModelPrice } from '@/lib/types'

interface PriceView {
  prices: ModelPrice[]
  default_price: ModelPrice
  models: string[]
  cny_per_usd: number
  unit: string
}

const isEmptyDefault = (p: ModelPrice) => p.model === '*'

/** 单价（元/百万 Token）对应的美元参考价。 */
const usdPerMillion = (cnyPrice: number, rate: number) => (cnyPrice / (rate > 0 ? rate : 7.2)).toFixed(2)

export default function AdminPricing() {
  const qc = useQueryClient()
  const confirm = useConfirm()
  const q = useQuery({ queryKey: ['admin', 'prices'], queryFn: () => api<PriceView>('/admin/api/prices') })
  const [search, setSearch] = useState('')
  const [editing, setEditing] = useState<ModelPrice | null>(null)
  const [model, setModel] = useState('')
  const [input, setInput] = useState('')
  const [output, setOutput] = useState('')
  const [cacheRead, setCacheRead] = useState('')
  const [busy, setBusy] = useState(false)

  const rate = q.data?.cny_per_usd ?? 7.2
  const rows = useMemo(() => {
    const list = (q.data?.prices || []).filter((p) => !isEmptyDefault(p))
    const s = search.trim().toLowerCase()
    return s ? list.filter((p) => p.model.toLowerCase().includes(s)) : list
  }, [q.data, search])

  const startEdit = (p: ModelPrice | null) => {
    setEditing(p)
    setModel(p?.model || '')
    setInput(p ? String(p.input) : '')
    setOutput(p ? String(p.output) : '')
    setCacheRead(p && p.cache_read >= 0 ? String(p.cache_read) : '')
  }

  const save = async (e: React.FormEvent) => {
    e.preventDefault()
    const m = model.trim()
    if (!m) {
      toast.error('请填写模型 ID')
      return
    }
    const inV = Number(input)
    const outV = Number(output)
    if (!(inV >= 0) || !(outV >= 0) || input === '' || output === '') {
      toast.error('输入、输出单价必须是非负数字')
      return
    }
    setBusy(true)
    try {
      await api('/admin/api/prices', {
        method: 'PUT',
        json: { model: m, input: inV, output: outV, cache_read: cacheRead === '' ? -1 : Math.max(0, Number(cacheRead) || 0) },
      })
      toast.success(`${m} 单价已保存`)
      startEdit(null)
      qc.invalidateQueries({ queryKey: ['admin', 'prices'] })
    } catch (err) {
      toast.error((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const remove = async (p: ModelPrice) => {
    if (!(await confirm({ title: `删除 ${p.model} 的单价？`, description: '删除后该模型回落到默认单价。', confirmText: '删除', destructive: true }))) return
    try {
      await api(`/admin/api/prices/${encodeURIComponent(p.model)}`, { method: 'DELETE' })
      toast.success('已删除')
      qc.invalidateQueries({ queryKey: ['admin', 'prices'] })
    } catch (err) {
      toast.error((err as Error).message)
    }
  }

  return (
    <>
      <PageHeader
        title="计费"
        description="按模型设置人民币单价（元 / 百万 Token）。每次请求按 未缓存输入、缓存输入、输出 分别计价后乘以倍率，余额与流水均以人民币结算。"
      />
      {q.isError ? (
        <Section>
          <ErrorState error={q.error} onRetry={() => q.refetch()} />
        </Section>
      ) : (
        <div className="space-y-4">
          <Section
            title="默认单价"
            description="未单独配置的模型使用这组价格；缓存价留空表示按输入价计费。"
            actions={
              <Button size="sm" variant="outline" disabled={!q.data} onClick={() => q.data && startEdit(q.data.default_price)}>
                <Pencil /> 修改默认价
              </Button>
            }
          >
            {q.data ? (
              <div className="grid gap-3 p-4 sm:grid-cols-3">
                <DefaultPrice label="输入" value={q.data.default_price.input} cache={q.data.default_price.cache_read} rate={rate} />
                <DefaultPrice label="输出" value={q.data.default_price.output} rate={rate} />
                <DefaultPrice label="缓存命中" value={q.data.default_price.cache_read} rate={rate} empty="同输入价" />
              </div>
            ) : (
              <div className="p-4">
                <TableSkeleton rows={1} cols={4} />
              </div>
            )}
          </Section>

          <Section
            title="模型单价"
            description={q.data ? `${rows.length} 个已配置 · 汇率 1 美元 ≈ ¥${rate}` : undefined}
            actions={
              <div className="flex items-center gap-2">
                <Input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="搜索已配置模型" className="h-7 w-40 text-xs" />
                <Button size="sm" variant="outline" onClick={() => startEdit(null)}>
                  <Plus /> 添加
                </Button>
              </div>
            }
          >
            <form className="grid gap-3 border-b p-4 sm:grid-cols-2 lg:grid-cols-5" onSubmit={save}>
              <Field label="模型 ID" htmlFor="p-model" hint={editing ? '正在编辑' : '可输入目录外的模型'}>
                <Input id="p-model" list="price-models" value={model} onChange={(e) => setModel(e.target.value)} placeholder="例如 gemini-3.5-pro" disabled={!!editing} />
                <datalist id="price-models">
                  {(q.data?.models || []).map((m) => (
                    <option key={m} value={m} />
                  ))}
                </datalist>
              </Field>
              <Field label="输入价（元 / 百万）" htmlFor="p-in">
                <Input id="p-in" inputMode="decimal" value={input} onChange={(e) => setInput(e.target.value.replace(/[^\d.]/g, ''))} placeholder="2.00" />
              </Field>
              <Field label="输出价（元 / 百万）" htmlFor="p-out">
                <Input id="p-out" inputMode="decimal" value={output} onChange={(e) => setOutput(e.target.value.replace(/[^\d.]/g, ''))} placeholder="8.00" />
              </Field>
              <Field label="缓存价（元 / 百万）" htmlFor="p-cache" hint="留空 = 同输入价">
                <Input id="p-cache" inputMode="decimal" value={cacheRead} onChange={(e) => setCacheRead(e.target.value.replace(/[^\d.]/g, ''))} placeholder="同输入价" />
              </Field>
              <div className="flex items-end gap-2">
                <Button type="submit" disabled={busy} className="flex-1">
                  <Check /> {editing ? '保存' : '添加'}
                </Button>
                {editing && (
                  <Button type="button" variant="ghost" size="icon" onClick={() => startEdit(null)} aria-label="取消">
                    <X />
                  </Button>
                )}
              </div>
            </form>

            {q.isLoading ? (
              <TableSkeleton rows={5} cols={5} />
            ) : !rows.length ? (
              <EmptyState icon={<Pencil />} title="还没有单独配置的模型" description="所有模型都按默认单价计费；也可以在上方为常用模型单独定价。" />
            ) : (
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead>模型</TableHead>
                    <TableHead className="text-right">输入</TableHead>
                    <TableHead className="text-right">输出</TableHead>
                    <TableHead className="text-right">缓存命中</TableHead>
                    <TableHead className="text-right">更新</TableHead>
                    <TableHead className="w-20" />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {rows.map((p) => (
                    <TableRow key={p.model}>
                      <TableCell className="max-w-[280px] truncate font-mono text-xs">{p.model}</TableCell>
                      <PriceCell cny={p.input} rate={rate} />
                      <PriceCell cny={p.output} rate={rate} />
                      <PriceCell cny={p.cache_read} rate={rate} empty={p.cache_read < 0 ? '同输入价' : undefined} />
                      <TableCell className="text-right text-xs text-muted-foreground">{p.updated_at ? dateTime(p.updated_at) : '—'}</TableCell>
                      <TableCell>
                        <div className="flex justify-end gap-1">
                          <Button size="icon-sm" variant="ghost" onClick={() => startEdit(p)} aria-label="编辑">
                            <Pencil />
                          </Button>
                          <Button size="icon-sm" variant="ghost" onClick={() => remove(p)} aria-label="删除">
                            <Trash2 />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
            <p className="px-4 py-3 text-[11px] leading-relaxed text-muted-foreground">
              费用 = （未缓存输入 × 输入价 + 缓存输入 × 缓存价 + 输出 × 输出价）× 模型倍率 × Key 倍率 × 用户倍率。1 元 / 百万 Token 等于 1 微元 / Token，余额与流水以微元整数存储。
            </p>
          </Section>
        </div>
      )}
    </>
  )
}

function DefaultPrice({ label, value, rate, cache, empty }: { label: string; value: number; rate: number; cache?: number; empty?: string }) {
  const shown = cache !== undefined && cache < 0 ? null : value
  return (
    <div className="rounded-md border px-3 py-2.5">
      <div className="text-[11px] text-muted-foreground">{label}</div>
      {shown === null ? (
        <div className="mt-0.5 text-[13px] text-muted-foreground">{empty || '—'}</div>
      ) : (
        <div className="mt-0.5 text-[15px] font-medium tabular-nums">
          ¥{shown} <span className="text-[11px] font-normal text-muted-foreground">/ 百万 Token · ≈ ${usdPerMillion(shown, rate)}</span>
        </div>
      )}
    </div>
  )
}

function PriceCell({ cny, rate, empty }: { cny: number; rate: number; empty?: string }) {
  if (empty) return <TableCell className="text-right text-xs text-muted-foreground">{empty}</TableCell>
  return (
    <TableCell className="whitespace-nowrap text-right tabular-nums">
      ¥{cny}
      <div className="text-[11px] text-muted-foreground">≈ ${usdPerMillion(cny, rate)}</div>
    </TableCell>
  )
}
