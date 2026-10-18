import { Button } from '@/components/ui/button'
import { Pill } from '@/components/status'
import { cn } from '@/lib/utils'

export const modelKinds = [
  { id: 'all', label: '全部' },
  { id: 'chat', label: '对话' },
  { id: 'image', label: '生图' },
  { id: 'video', label: '视频' },
  { id: 'music', label: '音乐' },
  { id: 'speech', label: '语音' },
  { id: 'embedding', label: '向量' },
] as const

const kindTone = {
  chat: 'muted',
  image: 'primary',
  video: 'primary',
  music: 'success',
  speech: 'warning',
  embedding: 'muted',
} as const

export function modelKindLabel(kind?: string) {
  return modelKinds.find((item) => item.id === kind)?.label || '对话'
}

export function ModelKindPill({ kind }: { kind?: string }) {
  if (!kind) return null
  return <Pill tone={kindTone[kind as keyof typeof kindTone] || 'muted'}>{modelKindLabel(kind)}</Pill>
}

export function ModelKindFilter({ value, onChange }: { value: string; onChange: (value: string) => void }) {
  return (
    <div className="flex flex-wrap gap-1">
      {modelKinds.map((item) => (
        <Button key={item.id} type="button" size="xs" variant={value === item.id ? 'default' : 'outline'} className={cn(value === item.id && 'pointer-events-none')} onClick={() => onChange(item.id)}>
          {item.label}
        </Button>
      ))}
    </div>
  )
}
