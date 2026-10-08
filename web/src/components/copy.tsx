import { useState } from 'react'
import { Check, Copy } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

export async function copyText(text: string, label = '已复制') {
  try {
    await navigator.clipboard.writeText(text)
  } catch {
    const ta = document.createElement('textarea')
    ta.value = text
    document.body.appendChild(ta)
    ta.select()
    document.execCommand('copy')
    ta.remove()
  }
  toast.success(label)
}

export function CopyButton({ value, label, className }: { value: string; label?: string; className?: string }) {
  const [done, setDone] = useState(false)
  return (
    <Button
      type="button"
      variant="ghost"
      size="icon-sm"
      className={cn('text-muted-foreground hover:text-foreground', className)}
      aria-label="复制"
      onClick={async (e) => {
        e.stopPropagation()
        await copyText(value, label)
        setDone(true)
        setTimeout(() => setDone(false), 1200)
      }}
    >
      {done ? <Check className="text-success" /> : <Copy />}
    </Button>
  )
}
