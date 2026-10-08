import { ChevronLeft, ChevronRight } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { num } from '@/lib/format'

export function Pager({ total, limit, offset, onChange }: { total: number; limit: number; offset: number; onChange: (offset: number) => void }) {
  if (total <= limit && offset === 0) {
    return total > 0 ? <div className="border-t px-3 py-2 text-xs text-muted-foreground">共 {num(total)} 条</div> : null
  }
  const page = Math.floor(offset / limit) + 1
  const pages = Math.max(1, Math.ceil(total / limit))
  return (
    <div className="flex items-center justify-between border-t px-3 py-2 text-xs text-muted-foreground">
      <span>
        共 {num(total)} 条 · 第 {page} / {pages} 页
      </span>
      <div className="flex items-center gap-1">
        <Button variant="ghost" size="icon-sm" disabled={offset === 0} onClick={() => onChange(Math.max(0, offset - limit))} aria-label="上一页">
          <ChevronLeft />
        </Button>
        <Button variant="ghost" size="icon-sm" disabled={offset + limit >= total} onClick={() => onChange(offset + limit)} aria-label="下一页">
          <ChevronRight />
        </Button>
      </div>
    </div>
  )
}
