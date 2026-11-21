import { useState } from 'react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { ScrollText, Search } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { KV, PageHeader, Section, Toolbar } from '@/components/page'
import { Pill } from '@/components/status'
import { EmptyState, ErrorState, TableSkeleton } from '@/components/states'
import { Pager } from '@/components/pager'
import { MiniSelect } from '@/components/fields'
import { useDebounced } from '@/hooks/use-misc'
import { api, qs } from '@/lib/api'
import { clock, dateTime, num } from '@/lib/format'
import type { AuditEntry } from '@/lib/types'

const LIMIT = 50

const rangeOptions = [
  { value: '1', label: '近 24 小时' },
  { value: '7', label: '近 7 天' },
  { value: '30', label: '近 30 天' },
  { value: '90', label: '近 90 天' },
]

/** 动作标识 → 中文标签。列表里同时保留原始 action，便于按前缀筛选。 */
export const auditActionLabels: Record<string, string> = {
  'auth.login': '登录',
  'auth.logout': '退出登录',
  'user.register': '用户注册',
  'user.create': '创建用户',
  'user.update': '修改用户',
  'user.delete': '删除用户',
  'user.password': '重置密码',
  'user.balance': '调整余额',
  'user.profile.update': '修改昵称',
  'user.password.change': '修改密码',
  'user.redeem': '兑换充值码',
  'user.key.create': '创建自己的 Key',
  'user.key.update': '修改自己的 Key',
  'user.key.delete': '删除自己的 Key',
  'user.key.regenerate': '重置自己的 Key',
  'key.create': '创建 Key',
  'key.update': '修改 Key',
  'key.delete': '删除 Key',
  'key.regenerate': '重新生成 Key',
  'account.add': '新增账号',
  'account.update': '启停账号',
  'account.credentials': '更新凭据',
  'account.delete': '删除账号',
  'account.check': '检测账号',
  'account.checkin': '账号签到',
  'account.auto_checkin': '自动签到',
  'account.oauth': 'OAuth 授权',
  'model.refresh': '刷新模型目录',
  'model.update': '上下架模型',
  'model.check': '模型检查',
  'model.batch': '批量上下架',
  'price.update': '修改单价',
  'price.delete': '删除单价',
  'multiplier.update': '修改倍率',
  'multiplier.delete': '删除倍率',
  'routing.update': '修改路由',
  'routing.group_update': '修改路由组',
  'usage.refund': '请求退款',
  'code.create': '生成兑换码',
  'code.update': '修改兑换码',
  'code.delete': '删除兑换码',
  'code.batch_disable': '整批停用',
  'settings.update': '修改设置',
  'upgrade.start': '触发升级',
}

const actionOptions = [
  { value: 'all', label: '全部动作' },
  { value: 'auth.', label: '登录与退出' },
  { value: 'user.', label: '用户' },
  { value: 'key.', label: 'API Key' },
  { value: 'account.', label: '账号' },
  { value: 'model.', label: '模型' },
  { value: 'price.', label: '计费单价' },
  { value: 'multiplier.', label: '倍率' },
  { value: 'routing.', label: '路由' },
  { value: 'usage.', label: '用量与退款' },
  { value: 'code.', label: '兑换码' },
  { value: 'settings.', label: '设置' },
  { value: 'upgrade.', label: '升级' },
]

const kindLabels: Record<string, string> = {
  root: '根管理员',
  admin: '管理员',
  user: '用户',
  system: '系统',
  anonymous: '匿名',
}

function actionLabel(action: string) {
  return auditActionLabels[action] || action
}

export default function AdminAudit() {
  const [days, setDays] = useState('7')
  const [action, setAction] = useState('all')
  const [result, setResult] = useState('all')
  const [search, setSearch] = useState('')
  const q = useDebounced(search.trim())
  const [offset, setOffset] = useState(0)
  const [selected, setSelected] = useState<AuditEntry | null>(null)

  const list = useQuery({
    queryKey: ['admin', 'audit', days, action, result, q, offset],
    queryFn: () => api<{ total: number; records: AuditEntry[]; days: number }>(
      `/admin/api/audit-logs${qs({ days, action: action === 'all' ? '' : action, result: result === 'all' ? '' : result, q, limit: LIMIT, offset })}`,
    ),
    placeholderData: keepPreviousData,
  })
  const d = list.data

  return (
    <>
      <PageHeader title="审计日志" description="管理员与用户的关键操作留痕：谁、什么时候、对谁、做了什么、结果如何。不记录密码与凭据明文。" />
      <Toolbar>
        <div className="relative">
          <Search className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={search}
            onChange={(e) => { setSearch(e.target.value); setOffset(0) }}
            placeholder="搜索操作人、对象、说明或错误"
            className="h-7 w-64 pl-8 text-xs"
          />
        </div>
        <MiniSelect value={days} onChange={(v) => { setDays(v); setOffset(0) }} options={rangeOptions} />
        <MiniSelect value={action} onChange={(v) => { setAction(v); setOffset(0) }} options={actionOptions} />
        <MiniSelect
          value={result}
          onChange={(v) => { setResult(v); setOffset(0) }}
          options={[{ value: 'all', label: '全部结果' }, { value: 'ok', label: '成功' }, { value: 'fail', label: '失败' }]}
        />
      </Toolbar>
      <Section>
        {list.isLoading ? (
          <TableSkeleton rows={8} cols={6} />
        ) : list.isError ? (
          <ErrorState error={list.error} onRetry={() => list.refetch()} />
        ) : !d?.records.length ? (
          <EmptyState icon={<ScrollText />} title="没有符合条件的审计记录" description="调整时间范围、动作或结果筛选。" />
        ) : (
          <>
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead>时间</TableHead>
                    <TableHead>操作人</TableHead>
                    <TableHead>动作</TableHead>
                    <TableHead className="hidden md:table-cell">对象</TableHead>
                    <TableHead className="hidden lg:table-cell">说明</TableHead>
                    <TableHead>结果</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {d.records.map((e) => (
                    <TableRow key={e.id} className="cursor-pointer" onClick={() => setSelected(e)}>
                      <TableCell className="whitespace-nowrap text-muted-foreground tabular-nums">
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <span>{clock(e.ts)}</span>
                          </TooltipTrigger>
                          <TooltipContent>{dateTime(e.ts)}</TooltipContent>
                        </Tooltip>
                      </TableCell>
                      <TableCell className="max-w-[180px] truncate" title={e.actor}>
                        <div className="flex items-center gap-1.5">
                          <span className="truncate">{e.actor || '—'}</span>
                          <Pill tone={e.actor_kind === 'root' || e.actor_kind === 'admin' ? 'primary' : 'muted'}>
                            {kindLabels[e.actor_kind] || e.actor_kind}
                          </Pill>
                        </div>
                      </TableCell>
                      <TableCell>
                        <div className="text-[13px]">{actionLabel(e.action)}</div>
                        <div className="font-mono text-[11px] text-muted-foreground">{e.action}</div>
                      </TableCell>
                      <TableCell className="hidden max-w-[220px] truncate font-mono text-xs text-muted-foreground md:table-cell" title={e.target}>
                        {e.target || '—'}
                      </TableCell>
                      <TableCell className="hidden max-w-[280px] truncate text-muted-foreground lg:table-cell" title={e.error || e.detail}>
                        {e.result === 'fail' && e.error ? e.error : e.detail || '—'}
                      </TableCell>
                      <TableCell>
                        {e.result === 'fail' ? <Pill tone="danger">失败</Pill> : <Pill tone="success">成功</Pill>}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
            <Pager total={d.total} limit={LIMIT} offset={offset} onChange={setOffset} />
          </>
        )}
      </Section>

      <Sheet open={!!selected} onOpenChange={(open) => !open && setSelected(null)}>
        <SheetContent className="w-full gap-0 sm:max-w-[440px]">
          {selected && (
            <>
              <SheetHeader className="border-b">
                <SheetTitle className="flex items-center gap-2 text-[15px]">
                  审计 #{selected.id}
                  {selected.result === 'fail' ? <Pill tone="danger">失败</Pill> : <Pill tone="success">成功</Pill>}
                </SheetTitle>
                <SheetDescription className="text-xs">{dateTime(selected.ts)}</SheetDescription>
              </SheetHeader>
              <div className="space-y-5 overflow-y-auto p-4">
                <KV
                  items={[
                    ['动作', <span>{actionLabel(selected.action)} <span className="font-mono text-[11px] text-muted-foreground">{selected.action}</span></span>],
                    ['操作人', selected.actor || '—'],
                    ['身份', kindLabels[selected.actor_kind] || selected.actor_kind],
                    ['来源 IP', <span className="font-mono text-xs">{selected.ip || '—'}</span>],
                    ['对象', <span className="font-mono text-xs">{selected.target || '—'}</span>],
                    ['说明', selected.detail || '—'],
                  ]}
                />
                {selected.error && (
                  <div>
                    <div className="mb-1.5 text-xs font-medium">错误信息</div>
                    <pre className="max-h-64 overflow-auto rounded-md border border-destructive/20 bg-destructive/5 p-2.5 font-mono text-[11px] leading-relaxed whitespace-pre-wrap text-[#ff9a9d]">{selected.error}</pre>
                  </div>
                )}
                <p className="text-xs text-muted-foreground">共 {num(list.data?.total)} 条匹配记录 · 审计记录不可修改或删除。</p>
              </div>
            </>
          )}
        </SheetContent>
      </Sheet>
    </>
  )
}
