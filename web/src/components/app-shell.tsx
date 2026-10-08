import { useEffect, useState, type ReactNode } from 'react'
import { NavLink, Outlet, useLocation, useNavigate } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { ArrowLeftRight, ChevronsUpDown, LogOut, Menu, Search } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { CommandPalette } from '@/components/command-palette'
import { useAuth, hasConsole, isAdmin } from '@/lib/auth'
import { api } from '@/lib/api'
import { compact } from '@/lib/format'
import type { PublicConfig } from '@/lib/types'
import { cn } from '@/lib/utils'
import type { NavGroup } from './nav'

export function usePublicConfig() {
  return useQuery({ queryKey: ['public-config'], queryFn: () => api<PublicConfig>('/api/public/config'), staleTime: 60_000 })
}

function Brand({ area }: { area: 'admin' | 'console' }) {
  const { data } = usePublicConfig()
  return (
    <div className="flex items-center gap-2.5 px-1">
      <div className="flex size-6 items-center justify-center rounded-md bg-gradient-to-b from-[#7c85f2] to-[#4f58c9] text-[11px] font-bold text-white shadow-[inset_0_1px_0_rgb(255_255_255/0.25)]">
        W
      </div>
      <div className="min-w-0 leading-tight">
        <div className="truncate text-[13px] font-semibold text-foreground">{data?.site_name || 'web2api'}</div>
        <div className="text-[11px] text-muted-foreground">{area === 'admin' ? '管理后台' : '控制台'}</div>
      </div>
    </div>
  )
}

function SidebarNav({ groups, onNavigate }: { groups: NavGroup[]; onNavigate?: () => void }) {
  return (
    <nav className="flex flex-col gap-4">
      {groups.map((g, i) => (
        <div key={i}>
          {g.label && <div className="mb-1 px-2 text-[11px] font-medium text-muted-foreground/80">{g.label}</div>}
          <div className="flex flex-col gap-px">
            {g.items.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.end}
                onClick={onNavigate}
                className={({ isActive }) =>
                  cn(
                    'group flex h-7 items-center gap-2.5 rounded-md px-2 text-[13px] text-sidebar-foreground transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground',
                    isActive && 'bg-sidebar-accent text-sidebar-accent-foreground shadow-[inset_0_0_0_1px_rgb(255_255_255/0.04)]',
                  )
                }
              >
                {({ isActive }) => (
                  <>
                    <item.icon className={cn('size-4 shrink-0 text-muted-foreground group-hover:text-foreground', isActive && 'text-primary')} />
                    <span className="truncate">{item.label}</span>
                  </>
                )}
              </NavLink>
            ))}
          </div>
        </div>
      ))}
    </nav>
  )
}

function UserMenu({ area }: { area: 'admin' | 'console' }) {
  const { me, logout } = useAuth()
  const navigate = useNavigate()
  if (!me) return null
  const initial = (me.nickname || me.email).slice(0, 1).toUpperCase()
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left transition-colors hover:bg-sidebar-accent">
          <div className="flex size-6 shrink-0 items-center justify-center rounded-full bg-white/[0.08] text-[11px] font-medium">{initial}</div>
          <div className="min-w-0 flex-1 leading-tight">
            <div className="truncate text-[13px]">{me.nickname || me.email}</div>
            <div className="truncate text-[11px] text-muted-foreground">
              {me.kind === 'user' ? `余额 ${compact(me.balance)}` : '初始化管理员'}
            </div>
          </div>
          <ChevronsUpDown className="size-3.5 text-muted-foreground" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent side="top" align="start" className="w-[220px]">
        <DropdownMenuLabel className="text-xs font-normal text-muted-foreground">{me.email}</DropdownMenuLabel>
        <DropdownMenuSeparator />
        {area === 'admin' && hasConsole(me) && (
          <DropdownMenuItem onSelect={() => navigate('/console')}>
            <ArrowLeftRight /> 切换到我的控制台
          </DropdownMenuItem>
        )}
        {area === 'console' && isAdmin(me) && (
          <DropdownMenuItem onSelect={() => navigate('/admin')}>
            <ArrowLeftRight /> 切换到管理后台
          </DropdownMenuItem>
        )}
        <DropdownMenuItem
          onSelect={async () => {
            await logout()
            navigate('/login', { replace: true })
          }}
        >
          <LogOut /> 退出登录
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function SidebarBody({ groups, area, onNavigate }: { groups: NavGroup[]; area: 'admin' | 'console'; onNavigate?: () => void }) {
  const { data } = usePublicConfig()
  return (
    <div className="flex h-full flex-col">
      <div className="flex h-12 items-center px-3">
        <Brand area={area} />
      </div>
      <div className="flex-1 overflow-y-auto px-2 pt-2 pb-4">
        <SidebarNav groups={groups} onNavigate={onNavigate} />
      </div>
      <div className="border-t border-sidebar-border p-2">
        <UserMenu area={area} />
        {data?.version && <div className="px-2 pt-1.5 text-[11px] text-muted-foreground/70">版本 {data.version}</div>}
      </div>
    </div>
  )
}

export function AppShell({ groups, area, children }: { groups: NavGroup[]; area: 'admin' | 'console'; children?: ReactNode }) {
  const [mobileOpen, setMobileOpen] = useState(false)
  const [paletteOpen, setPaletteOpen] = useState(false)
  const location = useLocation()
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        setPaletteOpen((v) => !v)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])
  useEffect(() => {
    document.querySelector('main')?.scrollTo({ top: 0 })
  }, [location.pathname])
  const current = groups.flatMap((g) => g.items).find((i) => (i.end ? location.pathname === i.to : location.pathname.startsWith(i.to)))
  const isMac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform)

  return (
    <div className="flex h-dvh overflow-hidden bg-background">
      <aside className="hidden w-[228px] shrink-0 border-r border-sidebar-border bg-sidebar md:block">
        <SidebarBody groups={groups} area={area} />
      </aside>
      <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
        <SheetContent side="left" className="w-[260px] border-sidebar-border bg-sidebar p-0">
          <SheetTitle className="sr-only">导航</SheetTitle>
          <SidebarBody groups={groups} area={area} onNavigate={() => setMobileOpen(false)} />
        </SheetContent>
      </Sheet>
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-12 shrink-0 items-center gap-2 border-b px-3 md:px-5">
          <Button variant="ghost" size="icon-sm" className="md:hidden" onClick={() => setMobileOpen(true)} aria-label="打开导航">
            <Menu />
          </Button>
          <div className="flex min-w-0 items-center gap-1.5 text-[13px]">
            <span className="text-muted-foreground">{area === 'admin' ? '管理后台' : '控制台'}</span>
            {current && (
              <>
                <span className="text-muted-foreground/50">/</span>
                <span className="truncate font-medium">{current.label}</span>
              </>
            )}
          </div>
          <div className="ml-auto flex items-center gap-2">
            <button
              onClick={() => setPaletteOpen(true)}
              className="flex h-7 items-center gap-2 rounded-md border bg-white/[0.02] px-2 text-xs text-muted-foreground transition-colors hover:bg-white/[0.05] hover:text-foreground sm:w-56"
            >
              <Search className="size-3.5" />
              <span className="hidden sm:inline">搜索或跳转…</span>
              <kbd className="ml-auto hidden rounded border bg-white/[0.04] px-1 font-mono text-[10px] sm:inline">{isMac ? '⌘' : 'Ctrl'} K</kbd>
            </button>
          </div>
        </header>
        <main className="flex-1 overflow-y-auto">
          <div className="mx-auto w-full max-w-[1240px] px-4 py-5 md:px-6 md:py-6">{children ?? <Outlet />}</div>
        </main>
      </div>
      <CommandPalette open={paletteOpen} onOpenChange={setPaletteOpen} groups={groups} area={area} />
    </div>
  )
}
