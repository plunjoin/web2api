import type { ReactNode } from 'react'
import { usePublicConfig } from '@/components/app-shell'

export function AuthLayout({ title, subtitle, children, footer }: { title: string; subtitle?: ReactNode; children: ReactNode; footer?: ReactNode }) {
  const { data } = usePublicConfig()
  return (
    <div className="relative flex min-h-dvh flex-col items-center justify-center overflow-hidden px-4 py-10">
      <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(60%_50%_at_50%_0%,rgb(110_120_230/0.14),transparent_70%)]" />
      <div className="pointer-events-none absolute inset-0 bg-[linear-gradient(rgb(255_255_255/0.025)_1px,transparent_1px),linear-gradient(90deg,rgb(255_255_255/0.025)_1px,transparent_1px)] bg-[size:48px_48px] [mask-image:radial-gradient(ellipse_at_center,black_30%,transparent_75%)]" />
      <div className="relative w-full max-w-[380px]">
        <div className="mb-6 flex flex-col items-center text-center">
          <div className="mb-4 flex size-10 items-center justify-center rounded-xl bg-gradient-to-b from-[#7c85f2] to-[#4f58c9] text-base font-bold text-white shadow-[0_8px_30px_rgb(110_120_230/0.35),inset_0_1px_0_rgb(255_255_255/0.25)]">
            W
          </div>
          <h1 className="text-[20px] font-semibold tracking-[-0.01em]">{title}</h1>
          {subtitle && <p className="mt-1.5 text-[13px] text-muted-foreground">{subtitle}</p>}
        </div>
        <div className="rounded-xl border bg-card/80 p-5 shadow-[0_20px_60px_-20px_rgb(0_0_0/0.6)] backdrop-blur">{children}</div>
        {footer && <div className="mt-4 text-center text-[13px] text-muted-foreground">{footer}</div>}
        <p className="mt-8 text-center text-[11px] text-muted-foreground/60">
          {data?.site_name || 'web2api'} {data?.version ? `· ${data.version}` : ''}
        </p>
      </div>
    </div>
  )
}

export function FormError({ message }: { message?: string }) {
  if (!message) return null
  return <p className="rounded-md border border-destructive/25 bg-destructive/10 px-3 py-2 text-xs text-[#ff8a8e]">{message}</p>
}
