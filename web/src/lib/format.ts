const nf = new Intl.NumberFormat('zh-CN')

/** 一元 = 1000000 微元；后端所有金额字段都是微元整数。 */
export const MICROS_PER_YUAN = 1_000_000

/** 金额小数位：≥¥1 两位，≥¥0.01 四位，更小六位。 */
function moneyDigits(v: number) {
  const abs = Math.abs(v)
  if (abs > 0 && abs < 0.01) return 6
  if (abs < 1) return 4
  return 2
}

function moneyNumber(v: number) {
  const digits = moneyDigits(v)
  return v.toLocaleString('zh-CN', { minimumFractionDigits: digits, maximumFractionDigits: digits })
}

/** 微元 → 人民币，如 ¥12.50 / ¥0.00276。 */
export const cny = (micros: number | null | undefined) => `¥${moneyNumber((micros ?? 0) / MICROS_PER_YUAN)}`

/** 直接格式化以元为单位的数值（如单价 元/百万 Token）。 */
export const yuan = (v: number) => `¥${moneyNumber(v)}`

/** 微元 → 美元参考价（按平台汇率折算）。 */
export const usd = (micros: number | null | undefined, rate = 7.2) =>
  `$${moneyNumber((micros ?? 0) / MICROS_PER_YUAN / (rate > 0 ? rate : 7.2))}`

/** 完整数字：1,234,567 */
export const num = (v: number | null | undefined) => nf.format(v ?? 0)

/** 紧凑数字：1.2万 / 3.4亿；小于 1 万显示完整数字 */
export function compact(v: number | null | undefined) {
  const n = v ?? 0
  const abs = Math.abs(n)
  if (abs >= 1e8) return `${trim(n / 1e8)}亿`
  if (abs >= 1e4) return `${trim(n / 1e4)}万`
  return nf.format(n)
}

/** 图表坐标轴：K / M */
export function axis(v: number) {
  const abs = Math.abs(v)
  if (abs >= 1e9) return `${trim(v / 1e9)}B`
  if (abs >= 1e6) return `${trim(v / 1e6)}M`
  if (abs >= 1e3) return `${trim(v / 1e3)}K`
  return String(v)
}

function trim(v: number) {
  return v.toFixed(v >= 100 || v <= -100 ? 0 : 1).replace(/\.0$/, '')
}

export const signed = (v: number) => (v > 0 ? `+${num(v)}` : num(v))

const pad = (n: number) => String(n).padStart(2, '0')

export function dateTime(ts: number | null | undefined) {
  if (!ts) return '—'
  const d = new Date(ts * 1000)
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** 时分秒：08:43:01（配合完整时间 tooltip） */
export function clock(ts: number | null | undefined) {
  if (!ts) return '—'
  const d = new Date(ts * 1000)
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

export function dateOnly(ts: number | null | undefined) {
  if (!ts) return '—'
  const d = new Date(ts * 1000)
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

export function relative(ts: number | null | undefined) {
  if (!ts) return '从未'
  const diff = Date.now() / 1000 - ts
  if (diff < 0) {
    const ahead = -diff
    if (ahead < 3600) return `${Math.ceil(ahead / 60)} 分钟后`
    if (ahead < 86400) return `${Math.ceil(ahead / 3600)} 小时后`
    return `${Math.ceil(ahead / 86400)} 天后`
  }
  if (diff < 45) return '刚刚'
  if (diff < 3600) return `${Math.floor(diff / 60)} 分钟前`
  if (diff < 86400) return `${Math.floor(diff / 3600)} 小时前`
  if (diff < 86400 * 30) return `${Math.floor(diff / 86400)} 天前`
  return dateOnly(ts)
}

export function duration(seconds: number) {
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (d > 0) return `${d} 天 ${h} 小时`
  if (h > 0) return `${h} 小时 ${m} 分`
  return `${m} 分钟`
}

export const latency = (ms: number) => (ms >= 1000 ? `${(ms / 1000).toFixed(1)}s` : `${ms}ms`)

export const multiplier = (v: number) => `×${Number(v.toFixed(4))}`

export function maskKey(key: string) {
  if (!key) return ''
  if (key.length <= 12) return key.slice(0, 3) + '…'
  return `${key.slice(0, 7)}…${key.slice(-4)}`
}

export const ledgerKindLabel: Record<string, string> = {
  adjust: '管理员调整',
  redeem: '兑换码充值',
  usage: '请求扣费',
  refund: '退款',
  signup_bonus: '注册赠送',
}

export const codeStatusLabel: Record<string, string> = {
  unused: '未使用',
  redeemed: '已兑换',
  disabled: '已停用',
  expired: '已过期',
}
