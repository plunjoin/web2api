import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { Pill } from '@/components/status'
import { dateTime, ledgerKindLabel, num, relative, signed } from '@/lib/format'
import type { LedgerEntry } from '@/lib/types'
import { cn } from '@/lib/utils'

const kindTone = { adjust: 'warning', redeem: 'success', usage: 'muted', refund: 'primary', signup_bonus: 'success' } as const

// compact：用于详情抽屉等窄容器——不显示「变动前」，说明折到类型下方。
// 非 compact 时按屏幕宽度逐步隐藏次要列，手机上同样把说明折到类型下方。
export function LedgerTable({ entries, showUser, onUserClick, compact }: { entries: LedgerEntry[]; showUser?: boolean; onUserClick?: (id: number) => void; compact?: boolean }) {
  const beforeCol = compact ? 'hidden' : 'hidden md:table-cell'
  const noteCol = compact ? 'hidden' : 'hidden sm:table-cell'
  const inlineNote = compact ? 'max-w-[220px]' : 'max-w-[130px] sm:hidden'
  const afterCol = compact ? '' : 'hidden sm:table-cell'
  const inlineAfter = compact ? 'hidden' : 'sm:hidden'
  return (
    <div className="overflow-x-auto">
      <Table>
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            <TableHead>时间</TableHead>
            {showUser && <TableHead>用户</TableHead>}
            <TableHead>类型</TableHead>
            <TableHead className="text-right">变动</TableHead>
            <TableHead className={cn('text-right', beforeCol)}>变动前</TableHead>
            <TableHead className={cn('text-right', afterCol)}>变动后</TableHead>
            <TableHead className={noteCol}>说明</TableHead>
            {showUser && <TableHead className="hidden lg:table-cell">操作人</TableHead>}
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
                <TableCell className="max-w-[180px] truncate">
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
                {e.note && (
                  <div className={cn('mt-1 truncate text-xs text-muted-foreground', inlineNote)} title={e.note}>
                    {e.note}
                  </div>
                )}
              </TableCell>
              <TableCell className={cn('text-right font-medium tabular-nums', e.amount > 0 ? 'text-success' : 'text-foreground')}>
                {signed(e.amount)}
                <div className={cn('mt-1 text-xs font-normal text-muted-foreground', inlineAfter)}>余额 {num(e.balance_after)}</div>
              </TableCell>
              <TableCell className={cn('text-right text-muted-foreground tabular-nums', beforeCol)}>{num(e.balance_before)}</TableCell>
              <TableCell className={cn('text-right tabular-nums', afterCol, e.balance_after < 0 && 'text-destructive')}>{num(e.balance_after)}</TableCell>
              <TableCell className={cn('truncate text-muted-foreground', showUser ? 'max-w-[240px]' : 'max-w-[320px]', noteCol)} title={e.note}>
                {e.note || '—'}
              </TableCell>
              {showUser && (
                <TableCell className="hidden max-w-[160px] truncate text-muted-foreground lg:table-cell" title={e.operator}>
                  {e.operator || '—'}
                </TableCell>
              )}
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
