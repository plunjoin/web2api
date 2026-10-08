import type { LucideIcon } from 'lucide-react'
import {
  BarChart3,
  BookOpen,
  Boxes,
  CreditCard,
  Gauge,
  KeyRound,
  LayoutDashboard,
  ReceiptText,
  Server,
  Settings,
  Ticket,
  UserCog,
  Users,
  Wallet,
} from 'lucide-react'

export interface NavItem {
  to: string
  label: string
  icon: LucideIcon
  keywords?: string
  end?: boolean
}

export interface NavGroup {
  label?: string
  items: NavItem[]
}

export const adminNav: NavGroup[] = [
  { items: [{ to: '/admin', label: '总览', icon: LayoutDashboard, end: true, keywords: 'overview dashboard zonglan' }] },
  {
    label: '运营',
    items: [
      { to: '/admin/users', label: '用户', icon: Users, keywords: 'users yonghu balance' },
      { to: '/admin/codes', label: '兑换码', icon: Ticket, keywords: 'redeem codes duihuanma' },
      { to: '/admin/ledger', label: '账单流水', icon: ReceiptText, keywords: 'ledger billing zhangdan' },
    ],
  },
  {
    label: '网关',
    items: [
      { to: '/admin/accounts', label: '号池账号', icon: Server, keywords: 'accounts pool gemini aistudio haochi' },
      { to: '/admin/keys', label: 'API Key', icon: KeyRound, keywords: 'keys api miyao' },
      { to: '/admin/usage', label: '用量', icon: BarChart3, keywords: 'usage yongliang records' },
      { to: '/admin/models', label: '模型与倍率', icon: Boxes, keywords: 'models multipliers pricing beilv' },
    ],
  },
  {
    label: '系统',
    items: [
      { to: '/admin/settings', label: '设置', icon: Settings, keywords: 'settings shezhi upgrade version registration' },
      { to: '/admin/docs', label: '接口文档', icon: BookOpen, keywords: 'docs api openapi wendang' },
    ],
  },
]

export const consoleNav: NavGroup[] = [
  { items: [{ to: '/console', label: '概览', icon: Gauge, end: true, keywords: 'dashboard overview' }] },
  {
    label: '使用',
    items: [
      { to: '/console/keys', label: 'API 密钥', icon: KeyRound, keywords: 'keys api' },
      { to: '/console/usage', label: '用量明细', icon: BarChart3, keywords: 'usage records' },
      { to: '/console/models', label: '模型与价格', icon: Boxes, keywords: 'models pricing' },
    ],
  },
  {
    label: '账户',
    items: [
      { to: '/console/billing', label: '账单', icon: Wallet, keywords: 'billing ledger balance' },
      { to: '/console/redeem', label: '兑换码', icon: CreditCard, keywords: 'redeem topup chongzhi' },
      { to: '/console/settings', label: '账号设置', icon: UserCog, keywords: 'profile password settings' },
    ],
  },
]
