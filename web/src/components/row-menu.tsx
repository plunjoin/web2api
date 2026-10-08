import type { ReactNode } from 'react'
import { MoreHorizontal } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'

export interface RowAction {
  label: string
  icon?: ReactNode
  onSelect: () => void
  destructive?: boolean
  disabled?: boolean
  separatorBefore?: boolean
}

/** 行尾“…”菜单：所有行内操作收进菜单，点击不会触发行点击。 */
export function RowMenu({ actions, label = '更多操作' }: { actions: RowAction[]; label?: string }) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild onClick={(e) => e.stopPropagation()}>
        <Button variant="ghost" size="icon-sm" className="text-muted-foreground data-[state=open]:bg-accent data-[state=open]:text-foreground" aria-label={label}>
          <MoreHorizontal />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-[168px]" onClick={(e) => e.stopPropagation()}>
        {actions.map((a, i) => (
          <div key={i}>
            {a.separatorBefore && <DropdownMenuSeparator />}
            <DropdownMenuItem
              disabled={a.disabled}
              variant={a.destructive ? 'destructive' : 'default'}
              onSelect={() => a.onSelect()}
              className="text-[13px]"
            >
              {a.icon}
              {a.label}
            </DropdownMenuItem>
          </div>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
