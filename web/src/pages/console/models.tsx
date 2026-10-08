import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Boxes, Search } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { PageHeader, Section, Toolbar } from '@/components/page'
import { EmptyState, ErrorState, TableSkeleton } from '@/components/states'
import { StatusDot } from '@/components/status'
import { CopyButton } from '@/components/copy'
import { api } from '@/lib/api'
import { multiplier, num } from '@/lib/format'

interface M {
  id: string
  display_name?: string
  available: boolean
  multiplier: number
}

export default function ConsoleModels() {
  const q = useQuery({ queryKey: ['user', 'models'], queryFn: () => api<{ models: M[]; user_multiplier: number; formula: string }>('/api/user/models') })
  const [search, setSearch] = useState('')
  const list = useMemo(() => (q.data?.models || []).filter((m) => !search || m.id.toLowerCase().includes(search.toLowerCase())), [q.data, search])
  return (
    <>
      <PageHeader title="模型与价格" description={q.data?.formula || '扣费 Token = ⌈总 Token × 模型倍率 × Key 倍率 × 用户倍率⌉'} />
      <Toolbar className="mb-3">
        <div className="relative">
          <Search className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="搜索模型" className="h-7 w-56 pl-8 text-xs" />
        </div>
        {q.data && <span className="text-xs text-muted-foreground">你的用户倍率 {multiplier(q.data.user_multiplier)}，已计入下表</span>}
      </Toolbar>
      <Section>
        {q.isLoading ? (
          <TableSkeleton rows={8} cols={4} />
        ) : q.isError ? (
          <ErrorState error={q.error} onRetry={() => q.refetch()} />
        ) : !list.length ? (
          <EmptyState icon={<Boxes />} title={search ? '没有匹配的模型' : '暂无可用模型'} description={search ? '换个关键词试试。' : '管理员还没有接入上游账号。'} />
        ) : (
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead>模型 ID</TableHead>
                  <TableHead>状态</TableHead>
                  <TableHead className="text-right">你的倍率</TableHead>
                  <TableHead className="text-right">1,000 Token 约扣</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {list.map((m) => (
                  <TableRow key={m.id}>
                    <TableCell>
                      <div className="flex items-center gap-1">
                        <span className="font-mono text-xs">{m.id}</span>
                        <CopyButton value={m.id} label="模型 ID 已复制" className="opacity-60 hover:opacity-100" />
                      </div>
                      {m.display_name && m.display_name !== m.id && <div className="text-[11px] text-muted-foreground">{m.display_name}</div>}
                    </TableCell>
                    <TableCell>{m.available ? <StatusDot tone="success">可用</StatusDot> : <StatusDot tone="muted">暂不可用</StatusDot>}</TableCell>
                    <TableCell className="text-right tabular-nums">{multiplier(m.multiplier)}</TableCell>
                    <TableCell className="text-right text-muted-foreground tabular-nums">{num(Math.ceil(1000 * m.multiplier))}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </Section>
    </>
  )
}
