import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ChevronRight, Download, FileText, Search } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { PageHeader, Section, Toolbar } from '@/components/page'
import { BlockSkeleton, EmptyState, ErrorState } from '@/components/states'
import { CopyButton } from '@/components/copy'
import { api } from '@/lib/api'
import { cn } from '@/lib/utils'

/* eslint-disable @typescript-eslint/no-explicit-any */
type Spec = { info: { title: string; description?: string; version: string }; tags?: { name: string; description?: string }[]; paths: Record<string, Record<string, any>>; components?: { schemas?: Record<string, any> } }
interface Op {
  method: string
  path: string
  tag: string
  summary: string
  op: any
}
const METHODS = ['get', 'post', 'put', 'patch', 'delete']
const methodTone: Record<string, string> = {
  get: 'text-[#5fb8ff] border-[#5fb8ff]/25 bg-[#5fb8ff]/10',
  post: 'text-success border-success/25 bg-success/10',
  put: 'text-warning border-warning/25 bg-warning/10',
  patch: 'text-[#c79bff] border-[#c79bff]/25 bg-[#c79bff]/10',
  delete: 'text-[#ff7a7e] border-destructive/25 bg-destructive/10',
}

const sources = {
  gateway: { label: '网关 API（/v1）', url: '/v1/docs', file: 'web2api-openapi.json' },
  admin: { label: '管理 API', url: '/admin/api/docs', file: 'web2api-admin-openapi.json' },
}

export default function AdminDocs() {
  const [source, setSource] = useState<keyof typeof sources>('gateway')
  const [search, setSearch] = useState('')
  const q = useQuery({ queryKey: ['docs', source], queryFn: () => api<Spec>(sources[source].url), staleTime: 300_000 })
  const spec = q.data
  const groups = useMemo(() => {
    if (!spec) return []
    const s = search.trim().toLowerCase()
    const ops: Op[] = []
    for (const [path, item] of Object.entries(spec.paths || {})) {
      for (const m of METHODS) {
        const op = item[m]
        if (!op) continue
        const summary = op.summary || op.operationId || ''
        if (s && !path.toLowerCase().includes(s) && !summary.toLowerCase().includes(s)) continue
        ops.push({ method: m, path, tag: op.tags?.[0] || '其他', summary, op })
      }
    }
    const order = (spec.tags || []).map((t) => t.name)
    const map = new Map<string, Op[]>()
    for (const o of ops) map.set(o.tag, [...(map.get(o.tag) || []), o])
    return [...map.entries()].sort((a, b) => (order.indexOf(a[0]) + 1 || 999) - (order.indexOf(b[0]) + 1 || 999))
  }, [spec, search])

  const exportSpec = () => {
    if (!spec) return
    const blob = new Blob([JSON.stringify(spec, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = sources[source].file
    a.click()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  }

  return (
    <>
      <PageHeader
        title="接口文档"
        description="OpenAPI 3.1，可导出后导入 Postman / Insomnia / Swagger UI。"
        actions={
          <Button variant="outline" onClick={exportSpec} disabled={!spec}>
            <Download /> 导出 JSON
          </Button>
        }
      />
      <Toolbar className="mb-3">
        <Tabs value={source} onValueChange={(v) => setSource(v as keyof typeof sources)}>
          <TabsList className="h-7">
            {Object.entries(sources).map(([k, v]) => (
              <TabsTrigger key={k} value={k} className="px-2.5 text-xs">
                {v.label}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
        <div className="relative">
          <Search className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="搜索路径或说明" className="h-7 w-56 pl-8 text-xs" />
        </div>
      </Toolbar>
      {q.isLoading ? (
        <BlockSkeleton className="h-96" />
      ) : q.isError ? (
        <Section>
          <ErrorState error={q.error} onRetry={() => q.refetch()} />
        </Section>
      ) : (
        <div className="space-y-4">
          {spec?.info.description && !search && <Intro title={spec.info.title} text={spec.info.description} />}
          {!groups.length && <Section><EmptyState icon={<FileText />} title="没有匹配的接口" /></Section>}
          {groups.map(([tag, ops]) => (
            <Section key={tag} title={tag} description={`${ops.length} 个接口${spec?.tags?.find((t) => t.name === tag)?.description ? ` · ${spec.tags.find((t) => t.name === tag)!.description}` : ''}`}>
              <div className="divide-y">
                {ops.map((o) => (
                  <Operation key={o.method + o.path} o={o} spec={spec!} />
                ))}
              </div>
            </Section>
          ))}
        </div>
      )}
    </>
  )
}

function Intro({ title, text }: { title: string; text: string }) {
  const [open, setOpen] = useState(false)
  return (
    <Section title={title} actions={<Button variant="ghost" size="sm" onClick={() => setOpen(!open)}>{open ? '收起' : '展开说明'}</Button>}>
      <div className={cn('relative overflow-hidden px-4 py-3', !open && 'max-h-28')}>
        <Markdownish text={text} />
        {!open && <div className="pointer-events-none absolute inset-x-0 bottom-0 h-12 bg-gradient-to-t from-card to-transparent" />}
      </div>
    </Section>
  )
}

/** 极简 Markdown：标题、表格、段落。文档是服务端生成的可信内容，但仍按纯文本渲染（不注入 HTML）。 */
function Markdownish({ text }: { text: string }) {
  const blocks = text.split(/\n{2,}/)
  return (
    <div className="space-y-2.5 text-[13px] leading-relaxed text-muted-foreground">
      {blocks.map((b, i) => {
        if (b.startsWith('## ')) return <h3 key={i} className="pt-1 text-[13px] font-semibold text-foreground">{b.slice(3)}</h3>
        if (b.trim().startsWith('|')) {
          const rows = b.trim().split('\n').filter((r) => !/^\|\s*-/.test(r)).map((r) => r.split('|').slice(1, -1).map((c) => c.trim()))
          return (
            <div key={i} className="overflow-x-auto rounded-md border">
              <table className="w-full text-xs">
                <tbody>
                  {rows.map((r, j) => (
                    <tr key={j} className={cn('border-b last:border-0', j === 0 && 'bg-white/[0.03] font-medium text-foreground')}>
                      {r.map((c, k) => <td key={k} className="px-2.5 py-1.5 align-top">{c}</td>)}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )
        }
        return <p key={i} className="whitespace-pre-wrap">{b}</p>
      })}
    </div>
  )
}

function resolve(spec: Spec, schema: any, depth = 0): any {
  if (!schema || depth > 4) return schema
  if (schema.$ref) return resolve(spec, spec.components?.schemas?.[schema.$ref.split('/').pop()], depth + 1)
  return schema
}
function typeOf(spec: Spec, s: any): string {
  if (!s) return ''
  if (s.$ref) return s.$ref.split('/').pop()
  if (s.type === 'array') return `${typeOf(spec, s.items)}[]`
  if (s.enum) return s.enum.map(String).join(' | ')
  if (Array.isArray(s.type)) return s.type.join(' | ')
  return s.type || (s.oneOf || s.anyOf ? 'oneOf' : 'object')
}

function Operation({ o, spec }: { o: Op; spec: Spec }) {
  const [open, setOpen] = useState(false)
  const op = o.op
  const content = op.requestBody?.content || {}
  const media = Object.keys(content)[0]
  const bodySchema = media ? resolve(spec, content[media].schema) : null
  const example = media ? content[media].example ?? Object.values(content[media].examples || {})[0] : undefined
  const exampleValue = example && typeof example === 'object' && 'value' in (example as any) ? (example as any).value : example
  return (
    <div>
      <button className="flex w-full items-center gap-3 px-4 py-2 text-left hover:bg-accent/40" onClick={() => setOpen(!open)} aria-expanded={open}>
        <ChevronRight className={cn('size-3.5 shrink-0 text-muted-foreground transition-transform', open && 'rotate-90')} />
        <span className={cn('inline-flex h-5 w-14 shrink-0 items-center justify-center rounded border font-mono text-[10px] font-semibold uppercase', methodTone[o.method])}>{o.method}</span>
        <code className="min-w-0 truncate font-mono text-xs">{o.path}</code>
        <span className="ml-auto hidden truncate text-xs text-muted-foreground sm:block">{o.summary}</span>
      </button>
      {open && (
        <div className="space-y-3 border-t bg-white/[0.015] px-4 py-3 pl-11">
          <div className="flex items-center gap-2">
            <code className="font-mono text-xs">{o.method.toUpperCase()} {o.path}</code>
            <CopyButton value={o.path} label="路径已复制" />
          </div>
          {o.summary && <div className="text-[13px] font-medium">{o.summary}</div>}
          {op.description && <Markdownish text={op.description} />}
          {op.parameters?.length > 0 && (
            <Block title="参数">
              {op.parameters.map((p: any, i: number) => {
                const pp = p.$ref ? resolve(spec, p) : p
                return <Prop key={i} name={pp.name} type={`${pp.in} · ${typeOf(spec, pp.schema)}`} required={pp.required} desc={pp.description} />
              })}
            </Block>
          )}
          {bodySchema?.properties && (
            <Block title={`请求体 · ${media}`}>
              {Object.entries(bodySchema.properties).map(([k, v]: [string, any]) => (
                <Prop key={k} name={k} type={typeOf(spec, v)} required={bodySchema.required?.includes(k)} desc={resolve(spec, v)?.description} />
              ))}
            </Block>
          )}
          {exampleValue !== undefined && (
            <Block title="示例">
              <pre className="max-h-64 overflow-auto p-3 font-mono text-[11px] leading-relaxed text-[#c9cbe0]">{JSON.stringify(exampleValue, null, 2)}</pre>
            </Block>
          )}
          {op.responses && (
            <Block title="响应">
              {Object.entries(op.responses).map(([code, r]: [string, any]) => (
                <Prop key={code} name={code} type="" desc={resolve(spec, r)?.description} />
              ))}
            </Block>
          )}
        </div>
      )}
    </div>
  )
}

function Block({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="overflow-hidden rounded-md border bg-card">
      <div className="border-b px-3 py-1.5 text-[11px] font-medium text-muted-foreground">{title}</div>
      <div className="divide-y">{children}</div>
    </div>
  )
}
function Prop({ name, type, required, desc }: { name: string; type: string; required?: boolean; desc?: string }) {
  return (
    <div className="grid gap-1 px-3 py-1.5 text-xs sm:grid-cols-[200px_1fr]">
      <div className="min-w-0">
        <code className="font-mono text-foreground">{name}</code>
        {required && <span className="ml-1 text-[10px] text-[#ff7a7e]">必填</span>}
        {type && <div className="truncate font-mono text-[10.5px] text-muted-foreground">{type}</div>}
      </div>
      <div className="whitespace-pre-wrap text-muted-foreground">{desc || '—'}</div>
    </div>
  )
}
