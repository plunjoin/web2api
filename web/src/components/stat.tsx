import type { ReactNode } from 'react'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

export interface StatItem {
  label: ReactNode
  value: ReactNode
  hint?: ReactNode
  icon?: ReactNode
  tone?: 'default' | 'success' | 'warning' | 'danger' | 'primary'
}

const toneClass = {
  default: 'text-foreground',
  success: 'text-success',
  warning: 'text-warning',
  danger: 'text-destructive',
  primary: 'text-primary',
}

/** 指标条：一个带边框的容器内按列分隔（而不是一堆浮在空白里的卡片）。 */
export function StatStrip({ items, loading, className }: { items: StatItem[]; loading?: boolean; className?: string }) {
  return (
    <div
      className={cn(
        'grid grid-cols-2 divide-y overflow-hidden rounded-lg border bg-card sm:divide-y-0',
        items.length >= 4 ? 'lg:grid-cols-4' : 'lg:grid-cols-3',
        items.length >= 4 ? 'sm:grid-cols-2' : 'sm:grid-cols-3',
        '[&>*]:border-border sm:[&>*:not(:first-child)]:border-l',
        className,
      )}
    >
      {items.map((item, i) => (
        <div key={i} className="min-w-0 px-4 py-3.5">
          <div className="flex items-center gap-1.5 text-xs text-muted-foreground [&_svg]:size-3.5">
            {item.icon}
            <span className="truncate">{item.label}</span>
          </div>
          {loading ? (
            <Skeleton className="mt-2 h-6 w-24" />
          ) : (
            <div className={cn('mt-1 truncate text-[22px] font-semibold tracking-[-0.02em] tabular-nums', toneClass[item.tone || 'default'])}>{item.value}</div>
          )}
          {item.hint && <div className="mt-0.5 truncate text-xs text-muted-foreground">{loading ? <Skeleton className="h-3 w-28" /> : item.hint}</div>}
        </div>
      ))}
    </div>
  )
}
