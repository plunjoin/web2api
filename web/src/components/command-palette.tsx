import { useNavigate } from 'react-router'
import { ArrowLeftRight, LogOut, Plus } from 'lucide-react'
import { CommandDialog, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList, CommandSeparator } from '@/components/ui/command'
import { useAuth, hasConsole, isAdmin } from '@/lib/auth'
import type { NavGroup } from './nav'

export function CommandPalette({ open, onOpenChange, groups, area }: { open: boolean; onOpenChange: (v: boolean) => void; groups: NavGroup[]; area: 'admin' | 'console' }) {
  const navigate = useNavigate()
  const { me, logout } = useAuth()
  const go = (to: string) => {
    onOpenChange(false)
    navigate(to)
  }
  const actions =
    area === 'admin'
      ? [
          { label: '新建用户', to: '/admin/users?new=1' },
          { label: '生成兑换码', to: '/admin/codes?new=1' },
          { label: '创建 API Key', to: '/admin/keys?new=1' },
          { label: '添加 Gemini 网页号', to: '/admin/accounts?new=a' },
          { label: '添加 AI Studio 号', to: '/admin/accounts?new=b' },
        ]
      : [
          { label: '创建 API 密钥', to: '/console/keys?new=1' },
          { label: '兑换充值码', to: '/console/redeem' },
        ]
  return (
    <CommandDialog open={open} onOpenChange={onOpenChange} title="命令面板" description="搜索页面或执行操作">
      <CommandInput placeholder="输入页面或操作名称…" />
      <CommandList>
        <CommandEmpty>没有匹配的结果</CommandEmpty>
        {groups.map((g, i) => (
          <CommandGroup key={i} heading={g.label || '导航'}>
            {g.items.map((item) => (
              <CommandItem key={item.to} value={`${item.label} ${item.keywords || ''}`} onSelect={() => go(item.to)}>
                <item.icon />
                {item.label}
              </CommandItem>
            ))}
          </CommandGroup>
        ))}
        <CommandSeparator />
        <CommandGroup heading="操作">
          {actions.map((a) => (
            <CommandItem key={a.to} value={a.label} onSelect={() => go(a.to)}>
              <Plus />
              {a.label}
            </CommandItem>
          ))}
          {area === 'admin' && hasConsole(me) && (
            <CommandItem value="切换到我的控制台 console" onSelect={() => go('/console')}>
              <ArrowLeftRight /> 切换到我的控制台
            </CommandItem>
          )}
          {area === 'console' && isAdmin(me) && (
            <CommandItem value="切换到管理后台 admin" onSelect={() => go('/admin')}>
              <ArrowLeftRight /> 切换到管理后台
            </CommandItem>
          )}
          <CommandItem
            value="退出登录 logout"
            onSelect={async () => {
              onOpenChange(false)
              await logout()
              navigate('/login', { replace: true })
            }}
          >
            <LogOut /> 退出登录
          </CommandItem>
        </CommandGroup>
      </CommandList>
    </CommandDialog>
  )
}
