import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { api, onUnauthorized, tokenStore } from './api'
import type { Me } from './types'

interface AuthState {
  me: Me | null
  loading: boolean
  login: (email: string, password: string) => Promise<Me>
  register: (email: string, password: string, nickname: string) => Promise<Me>
  logout: () => Promise<void>
  refresh: () => Promise<void>
  setToken: (token: string) => void
}

const AuthContext = createContext<AuthState | null>(null)

interface TokenResponse {
  access_token?: string
  user: Me
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [me, setMe] = useState<Me | null>(null)
  const [loading, setLoading] = useState(true)
  const qc = useQueryClient()

  const refresh = useCallback(async () => {
    if (!tokenStore.get()) {
      setMe(null)
      setLoading(false)
      return
    }
    try {
      const res = await api<{ user: Me }>('/api/auth/me')
      setMe(res.user)
    } catch {
      tokenStore.clear()
      setMe(null)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void refresh()
    return onUnauthorized(() => {
      tokenStore.clear()
      setMe(null)
      qc.clear()
    })
  }, [refresh, qc])

  const accept = useCallback(
    (res: TokenResponse) => {
      if (res.access_token) tokenStore.set(res.access_token)
      qc.clear()
      setMe(res.user)
      return res.user
    },
    [qc],
  )

  const value = useMemo<AuthState>(
    () => ({
      me,
      loading,
      refresh,
      login: async (email, password) => accept(await api<TokenResponse>('/api/auth/login', { method: 'POST', json: { email, password } })),
      register: async (email, password, nickname) =>
        accept(await api<TokenResponse>('/api/auth/register', { method: 'POST', json: { email, password, nickname } })),
      logout: async () => {
        try {
          await api('/api/auth/logout', { method: 'POST' })
        } catch {
          /* 会话已失效时忽略 */
        }
        tokenStore.clear()
        qc.clear()
        setMe(null)
      },
      setToken: (token) => tokenStore.set(token),
    }),
    [me, loading, refresh, accept, qc],
  )
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used inside AuthProvider')
  return ctx
}

export const isAdmin = (me: Me | null) => me?.role === 'admin'
/** 平台用户（有余额、可用控制台）。 */
export const hasConsole = (me: Me | null) => me?.kind === 'user'
