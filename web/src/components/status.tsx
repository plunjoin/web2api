import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

type Tone = 'success' | 'warning' | 'danger' | 'muted' | 'primary'

const dot: Record<Tone, string> = {
  success: 'bg-success',
  warning: 'bg-warning',
  danger: 'bg-destructive',
  muted: 'bg-muted-foreground/50',
  primary: 'bg-primary',
}
const pill: Record<Tone, string> = {
  success: 'border-success/20 bg-success/10 text-success',
  warning: 'border-warning/20 bg-warning/10 text-warning',
  danger: 'border-destructive/25 bg-destructive/10 text-[#ff7a7e]',
  muted: 'border-border bg-white/[0.03] text-muted-foreground',
  primary: 'border-primary/25 bg-primary/10 text-[#a5abf5]',
}

export function StatusDot({ tone, children, className }: { tone: Tone; children?: ReactNode; className?: string }) {
  return (
    <span className={cn('inline-flex items-center gap-1.5 text-[13px]', className)}>
      <span className={cn('size-1.5 shrink-0 rounded-full', dot[tone])} />
      {children}
    </span>
  )
}

export function Pill({ tone = 'muted', children, className }: { tone?: Tone; children: ReactNode; className?: string }) {
  return <span className={cn('inline-flex h-5 items-center gap-1 rounded-md border px-1.5 text-[11px] font-medium whitespace-nowrap', pill[tone], className)}>{children}</span>
}
