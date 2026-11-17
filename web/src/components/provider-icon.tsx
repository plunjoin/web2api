import { PROVIDER_ICONS } from '@/assets/providers.gen'
import { cn } from '@/lib/utils'

// 供应商图标全部来自 magpie 的 icons 目录（已拷进 src/assets/providers）。
// 图标清单由 `go run ./tools/web` 生成，构建时原样拷到 dist/assets/providers/，
// 由 Go 服务的 /admin/assets/ 直接提供；没有图标时退回文字头像。
const URLS: Record<string, string> = {}
for (const [name, file] of Object.entries(PROVIDER_ICONS)) {
  URLS[name] = `/admin/assets/providers/${file}`
}

const SIZES = { xs: 'size-4', sm: 'size-5', md: 'size-6', lg: 'size-7' } as const

function fallbackLabel(name?: string): string {
  const text = (name || '').trim()
  if (!text) return '?'
  // 中文取首字，拉丁字母取前两位
  if (/^[一-龥]/.test(text)) return text.slice(0, 1)
  return text.replace(/[^a-z0-9]/gi, '').slice(0, 2).toUpperCase() || '?'
}

/**
 * 供应商图标。彩色图标（*-color、png）直接显示原图；
 * 单色图标用 CSS mask 跟随 currentColor，暗色主题下不会变成一团黑。
 */
export function ProviderIcon({
  icon,
  name,
  size = 'sm',
  className,
}: {
  icon?: string
  name?: string
  size?: keyof typeof SIZES
  className?: string
}) {
  const url = icon ? URLS[icon] : undefined
  const box = cn('grid shrink-0 place-items-center', SIZES[size], className)

  if (!url) {
    return (
      <span
        aria-hidden={!name}
        title={name}
        className={cn(box, 'rounded-md bg-muted text-[9px] font-semibold text-muted-foreground')}
      >
        {fallbackLabel(name)}
      </span>
    )
  }

  if (icon?.endsWith('-color') || url.endsWith('.png')) {
    return <img src={url} alt="" title={name} draggable={false} className={cn(box, 'object-contain')} />
  }

  return (
    <span
      role="img"
      aria-label={name}
      title={name}
      className={cn(box, 'bg-current text-foreground/80')}
      style={{
        WebkitMaskImage: `url("${url}")`,
        maskImage: `url("${url}")`,
        WebkitMaskRepeat: 'no-repeat',
        maskRepeat: 'no-repeat',
        WebkitMaskPosition: 'center',
        maskPosition: 'center',
        WebkitMaskSize: 'contain',
        maskSize: 'contain',
      }}
    />
  )
}
