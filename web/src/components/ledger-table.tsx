import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { Pill } from '@/components/status'
import { dateTime, ledgerKindLabel, num, relative, signed } from '@/lib/format'
import type { LedgerEntry } from '@/lib/types'
import { cn } from '@/lib/utils'

const kindTone = { adjust: 'warning', redeem: 'success', usage: 'muted', refund: 'primary', signup_bonus: 'success' } as const

export function LedgerTable({ entries, showUser, onUserClick }: { entries: LedgerEntry[]; showUser?: boolean; onUserClick?: (id: number) => void }) {
  return (
    <div className="overflow-x-auto">
      <Table>
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            <TableHead>时间</TableHead>
            {showUser && <TableHead>用户</TableHead>}
            <TableHead>类型</TableHead>
            <TableHead className="text-right">变动</TableHead>
            <TableHead className="text-right">变动前</TableHead>
            <TableHead className="text-right">变动后</TableHead>
            <TableHead>说明</TableHead>
            {showUser && <TableHead>操作人</TableHead>}
          </TableRow>
        </TableHeader>
        <TableBody>
          {entries.map((e) => (
            <TableRow key={e.id}>
              <TableCell className="whitespace-nowrap text-muted-foreground">
                <Tooltip>
                  <TooltipTrigger asChild>
                    <span>{relative(e.ts)}</span>
                  </TooltipTrigger>
                  <TooltipContent>{dateTime(e.ts)}</TooltipContent>
                </Tooltip>
              </TableCell>
              {showUser && (
                <TableCell className="max-w-[200px] truncate">
                  {onUserClick ? (
                    <button className="hover:text-primary hover:underline" onClick={() => onUserClick(e.user_id)}>
                      {e.user_email || `#${e.user_id}`}
                    </button>
                  ) : (
                    e.user_email || `#${e.user_id}`
                  )}
                </TableCell>
              )}
              <TableCell>
                <Pill tone={kindTone[e.kind] || 'muted'}>{ledgerKindLabel[e.kind] || e.kind}</Pill>
              </TableCell>
              <TableCell className={cn('text-right font-medium tabular-nums', e.amount > 0 ? 'text-success' : 'text-foreground')}>{signed(e.amount)}</TableCell>
              <TableCell className="text-right text-muted-foreground tabular-nums">{num(e.balance_before)}</TableCell>
              <TableCell className={cn('text-right tabular-nums', e.balance_after < 0 && 'text-destructive')}>{num(e.balance_after)}</TableCell>
              <TableCell className="max-w-[320px] truncate text-muted-foreground" title={e.note}>
                {e.note || '—'}
              </TableCell>
              {showUser && <TableCell className="text-muted-foreground">{e.operator || '—'}</TableCell>}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}

export const ledgerKindOptions = [
  { value: 'all', label: '全部类型' },
  { value: 'redeem', label: '兑换充值' },
  { value: 'adjust', label: '管理员调整' },
  { value: 'usage', label: '使用扣费' },
  { value: 'refund', label: '退款' },
  { value: 'signup_bonus', label: '注册赠送' },
]
