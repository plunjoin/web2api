import { useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { PageHeader, Section, KV } from '@/components/page'
import { Field } from '@/components/fields'
import { api, tokenStore } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { dateTime, multiplier } from '@/lib/format'

export default function ConsoleSettings() {
  const { me, refresh } = useAuth()
  const [nickname, setNickname] = useState(me?.nickname || '')
  const [saving, setSaving] = useState(false)
  const [pw, setPw] = useState({ old: '', next: '', confirm: '' })
  const [pwBusy, setPwBusy] = useState(false)

  const saveProfile = async (e: React.FormEvent) => {
    e.preventDefault()
    setSaving(true)
    try {
      await api('/api/user/profile', { method: 'PATCH', json: { nickname: nickname.trim() } })
      await refresh()
      toast.success('资料已保存')
    } catch (err) {
      toast.error((err as Error).message)
    } finally {
      setSaving(false)
    }
  }
  const changePassword = async (e: React.FormEvent) => {
    e.preventDefault()
    if (pw.next !== pw.confirm) {
      toast.error('两次输入的新密码不一致')
      return
    }
    setPwBusy(true)
    try {
      const res = await api<{ access_token?: string }>('/api/user/password', { method: 'POST', json: { old_password: pw.old, new_password: pw.next } })
      if (res.access_token) tokenStore.set(res.access_token)
      setPw({ old: '', next: '', confirm: '' })
      toast.success('密码已修改，其他设备上的登录已失效')
    } catch (err) {
      toast.error((err as Error).message)
    } finally {
      setPwBusy(false)
    }
  }

  return (
    <>
      <PageHeader title="账号设置" description="管理个人资料与登录密码。" />
      <div className="grid max-w-3xl gap-4">
        <Section title="账号信息">
          <div className="p-4">
            <KV
              items={[
                ['邮箱', me?.email],
                ['角色', me?.role === 'admin' ? '管理员' : '普通用户'],
                ['用户倍率', multiplier(me?.multiplier ?? 1)],
                ['注册时间', dateTime(me?.created_at)],
              ]}
            />
          </div>
        </Section>
        <Section title="个人资料">
          <form onSubmit={saveProfile} className="flex flex-col gap-3 p-4 sm:flex-row sm:items-end">
            <Field label="昵称" htmlFor="nickname" className="flex-1">
              <Input id="nickname" value={nickname} maxLength={40} onChange={(e) => setNickname(e.target.value)} />
            </Field>
            <Button type="submit" disabled={saving || nickname.trim() === (me?.nickname || '')}>
              保存
            </Button>
          </form>
        </Section>
        <Section title="修改密码" description="修改后，其他设备上的登录会全部失效。">
          <form onSubmit={changePassword} className="grid gap-3 p-4 sm:max-w-sm">
            <Field label="当前密码" htmlFor="pw-old">
              <Input id="pw-old" type="password" autoComplete="current-password" value={pw.old} onChange={(e) => setPw({ ...pw, old: e.target.value })} />
            </Field>
            <Field label="新密码" htmlFor="pw-new" hint="至少 8 个字符。">
              <Input id="pw-new" type="password" autoComplete="new-password" value={pw.next} onChange={(e) => setPw({ ...pw, next: e.target.value })} />
            </Field>
            <Field label="确认新密码" htmlFor="pw-confirm">
              <Input id="pw-confirm" type="password" autoComplete="new-password" value={pw.confirm} onChange={(e) => setPw({ ...pw, confirm: e.target.value })} />
            </Field>
            <div>
              <Button type="submit" disabled={pwBusy || !pw.old || !pw.next}>
                修改密码
              </Button>
            </div>
          </form>
        </Section>
      </div>
    </>
  )
}
