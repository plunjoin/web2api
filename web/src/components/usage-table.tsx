import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { StatusDot } from '@/components/status'
import { dateTime, latency, num, relative } from '@/lib/format'
import type { UsageRecord } from '@/lib/types'

export function UsageTable({ records, onSelect, showUser }: { records: UsageRecord[]; onSelect: (r: UsageRecord) => void; showUser?: boolean }) {
  return (
    <div className="overflow-x-auto">
      <Table>
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            <TableHead>时间</TableHead>
            <TableHead>模型</TableHead>
            <TableHead>密钥</TableHead>
            {showUser && <TableHead>用户</TableHead>}
            <TableHead className="text-right">总 Token</TableHead>
            <TableHead className="text-right">扣费</TableHead>
            <TableHead className="text-right">耗时</TableHead>
            <TableHead>状态</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {records.map((r) => (
            <TableRow key={r.id} className="cursor-pointer" onClick={() => onSelect(r)}>
              <TableCell className="text-muted-foreground">
                <Tooltip>
                  <TooltipTrigger asChild>
                    <span>{relative(r.ts)}</span>
                  </TooltipTrigger>
                  <TooltipContent>{dateTime(r.ts)}</TooltipContent>
                </Tooltip>
              </TableCell>
              <TableCell className="font-mono text-xs">{r.model || '—'}</TableCell>
              <TableCell className="max-w-[180px] truncate text-muted-foreground">{r.key_name || r.key_masked || '—'}</TableCell>
              {showUser && <TableCell className="max-w-[180px] truncate text-muted-foreground">{r.user_email || (r.user_id ? `#${r.user_id}` : '—')}</TableCell>}
              <TableCell className="text-right tabular-nums">
                {num(r.total_tokens)}
                {r.estimated && <span className="ml-1 text-[11px] text-muted-foreground" title="本地估算">≈</span>}
              </TableCell>
              <TableCell className="text-right font-medium tabular-nums">
                {r.refunded_tokens > 0 ? <span className="text-muted-foreground line-through">{num(r.charged_tokens)}</span> : num(r.charged_tokens)}
              </TableCell>
              <TableCell className="text-right text-muted-foreground tabular-nums">{latency(r.latency_ms)}</TableCell>
              <TableCell>{r.success ? <StatusDot tone="success">成功</StatusDot> : <StatusDot tone="danger">失败</StatusDot>}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
