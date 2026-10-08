// 统一的 API 客户端：Bearer JWT（与旧管理台同一套登录会话），错误统一抛 ApiError。
const TOKEN_KEY = 'web2api_jwt'

export class ApiError extends Error {
  status: number
  data: unknown
  constructor(status: number, message: string, data?: unknown) {
    super(message)
    this.status = status
    this.data = data
  }
}

export const tokenStore = {
  get: () => localStorage.getItem(TOKEN_KEY) || '',
  set: (token: string) => localStorage.setItem(TOKEN_KEY, token),
  clear: () => localStorage.removeItem(TOKEN_KEY),
}

type Listener = () => void
const unauthorizedListeners = new Set<Listener>()
export function onUnauthorized(fn: Listener) {
  unauthorizedListeners.add(fn)
  return () => {
    unauthorizedListeners.delete(fn)
  }
}

export async function api<T = unknown>(path: string, init: RequestInit & { json?: unknown } = {}): Promise<T> {
  const headers = new Headers(init.headers)
  const token = tokenStore.get()
  if (token) headers.set('Authorization', `Bearer ${token}`)
  let body = init.body
  if (init.json !== undefined) {
    headers.set('Content-Type', 'application/json')
    body = JSON.stringify(init.json)
  }
  let res: Response
  try {
    res = await fetch(path, { ...init, headers, body })
  } catch {
    throw new ApiError(0, '网络连接失败，请检查服务是否在运行')
  }
  const text = await res.text()
  let data: unknown = undefined
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = text
    }
  }
  if (!res.ok) {
    const message =
      (data && typeof data === 'object' && 'error' in data && typeof (data as { error: unknown }).error === 'string'
        ? (data as { error: string }).error
        : undefined) || `请求失败（HTTP ${res.status}）`
    if (res.status === 401 && token) unauthorizedListeners.forEach((fn) => fn())
    throw new ApiError(res.status, message, data)
  }
  return data as T
}

/** 下载需要鉴权的文件（CSV 导出）。 */
export async function download(path: string, fallbackName: string) {
  const res = await fetch(path, { headers: { Authorization: `Bearer ${tokenStore.get()}` } })
  if (!res.ok) throw new ApiError(res.status, `导出失败（HTTP ${res.status}）`)
  const blob = await res.blob()
  const disposition = res.headers.get('Content-Disposition') || ''
  const name = /filename="([^"]+)"/.exec(disposition)?.[1] || fallbackName
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = name
  a.click()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

export function qs(params: Record<string, string | number | boolean | undefined | null>) {
  const search = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === null || v === '' || v === 'all') continue
    search.set(k, String(v))
  }
  const s = search.toString()
  return s ? `?${s}` : ''
}
