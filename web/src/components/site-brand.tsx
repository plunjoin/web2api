import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import type { PublicConfig } from '@/lib/types'
import { cn } from '@/lib/utils'

const DEFAULT_FAVICON =
  "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'%3E%3Crect width='32' height='32' rx='8' fill='%235e68d8'/%3E%3Ctext x='16' y='22' font-family='Arial' font-weight='700' font-size='16' fill='white' text-anchor='middle'%3EW%3C/text%3E%3C/svg%3E"

export function usePublicConfig() {
  return useQuery({ queryKey: ['public-config'], queryFn: () => api<PublicConfig>('/api/public/config'), staleTime: 60_000 })
}

/** 站点 Logo：配置了图片就用图片，加载失败或留空时回退到内置 W 标记。 */
export function SiteLogo({ logo, className }: { logo?: string; className?: string }) {
  const [failed, setFailed] = useState(false)
  useEffect(() => setFailed(false), [logo])
  if (logo && !failed) {
    return <img src={logo} alt="" className={cn('shrink-0 object-contain', className)} onError={() => setFailed(true)} />
  }
  return (
    <div
      aria-hidden="true"
      className={cn(
        'flex shrink-0 items-center justify-center bg-gradient-to-b from-[#7c85f2] to-[#4f58c9] font-bold text-white shadow-[inset_0_1px_0_rgb(255_255_255/0.25)]',
        className,
      )}
    >
      W
    </div>
  )
}

/** 把站点名与 Logo 同步到浏览器标题栏和 favicon。 */
export function SiteBranding() {
  const { data } = usePublicConfig()

  useEffect(() => {
    const name = data?.site_name?.trim()
    if (name) document.title = name
  }, [data?.site_name])

  useEffect(() => {
    const href = data?.site_logo?.trim() || DEFAULT_FAVICON
    let link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
    if (!link) {
      link = document.createElement('link')
      link.rel = 'icon'
      document.head.appendChild(link)
    }
    link.href = href
  }, [data?.site_logo])

  return null
}
