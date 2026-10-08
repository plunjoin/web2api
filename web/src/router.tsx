import { lazy, Suspense, type ReactNode } from 'react'
import { createBrowserRouter, Navigate, useLocation, useRouteError } from 'react-router'
import { AppShell } from '@/components/app-shell'
import { adminNav, consoleNav } from '@/components/nav'
import { ErrorState } from '@/components/states'
import { Skeleton } from '@/components/ui/skeleton'
import { useAuth, hasConsole, isAdmin } from '@/lib/auth'
import LoginPage from '@/pages/access/login'
import RegisterPage from '@/pages/access/register'

const ConsoleOverview = lazy(() => import('@/pages/console/overview'))
const ConsoleKeys = lazy(() => import('@/pages/console/keys'))
const ConsoleUsage = lazy(() => import('@/pages/console/usage'))
const ConsoleBilling = lazy(() => import('@/pages/console/billing'))
const ConsoleRedeem = lazy(() => import('@/pages/console/redeem'))
const ConsoleModels = lazy(() => import('@/pages/console/models'))
const ConsoleSettings = lazy(() => import('@/pages/console/settings'))
const AdminOverview = lazy(() => import('@/pages/admin/overview'))
const AdminUsers = lazy(() => import('@/pages/admin/users'))
const AdminCodes = lazy(() => import('@/pages/admin/codes'))
const AdminLedger = lazy(() => import('@/pages/admin/ledger'))
const AdminAccounts = lazy(() => import('@/pages/admin/accounts'))
const AdminKeys = lazy(() => import('@/pages/admin/keys'))
const AdminUsage = lazy(() => import('@/pages/admin/usage'))
const AdminModels = lazy(() => import('@/pages/admin/models'))
const AdminSettings = lazy(() => import('@/pages/admin/settings'))
const AdminDocs = lazy(() => import('@/pages/admin/docs'))

function PageFallback() {
  return (
    <div className="space-y-4">
      <Skeleton className="h-6 w-40" />
      <Skeleton className="h-4 w-72" />
      <Skeleton className="h-24 w-full" />
      <Skeleton className="h-64 w-full" />
    </div>
  )
}

function FullScreenLoading() {
  return (
    <div className="flex h-dvh items-center justify-center">
      <div className="size-5 animate-spin rounded-full border-2 border-white/10 border-t-primary" />
    </div>
  )
}

function Guard({ area, children }: { area: 'admin' | 'console'; children: ReactNode }) {
  const { me, loading } = useAuth()
  const location = useLocation()
  if (loading) return <FullScreenLoading />
  if (!me) return <Navigate to={`/login?next=${encodeURIComponent(location.pathname + location.search)}`} replace />
  if (area === 'admin' && !isAdmin(me)) return <Navigate to="/console" replace />
  if (area === 'console' && !hasConsole(me)) return <Navigate to="/admin" replace />
  return <>{children}</>
}

function Home() {
  const { me, loading } = useAuth()
  if (loading) return <FullScreenLoading />
  if (!me) return <Navigate to="/login" replace />
  return <Navigate to={isAdmin(me) ? '/admin' : '/console'} replace />
}

function RouteError() {
  const error = useRouteError()
  return (
    <div className="flex h-dvh items-center justify-center">
      <ErrorState error={error instanceof Error ? error : new Error('页面出错了')} onRetry={() => location.reload()} />
    </div>
  )
}

const page = (el: ReactNode) => <Suspense fallback={<PageFallback />}>{el}</Suspense>

export const router = createBrowserRouter([
  { path: '/', element: <Home />, errorElement: <RouteError /> },
  { path: '/login', element: <LoginPage />, errorElement: <RouteError /> },
  { path: '/register', element: <RegisterPage />, errorElement: <RouteError /> },
  {
    path: '/console',
    errorElement: <RouteError />,
    element: (
      <Guard area="console">
        <AppShell groups={consoleNav} area="console" />
      </Guard>
    ),
    children: [
      { index: true, element: page(<ConsoleOverview />) },
      { path: 'keys', element: page(<ConsoleKeys />) },
      { path: 'usage', element: page(<ConsoleUsage />) },
      { path: 'billing', element: page(<ConsoleBilling />) },
      { path: 'redeem', element: page(<ConsoleRedeem />) },
      { path: 'models', element: page(<ConsoleModels />) },
      { path: 'settings', element: page(<ConsoleSettings />) },
      { path: '*', element: <Navigate to="/console" replace /> },
    ],
  },
  {
    path: '/admin',
    errorElement: <RouteError />,
    element: (
      <Guard area="admin">
        <AppShell groups={adminNav} area="admin" />
      </Guard>
    ),
    children: [
      { index: true, element: page(<AdminOverview />) },
      { path: 'users', element: page(<AdminUsers />) },
      { path: 'codes', element: page(<AdminCodes />) },
      { path: 'ledger', element: page(<AdminLedger />) },
      { path: 'accounts', element: page(<AdminAccounts />) },
      { path: 'keys', element: page(<AdminKeys />) },
      { path: 'usage', element: page(<AdminUsage />) },
      { path: 'models', element: page(<AdminModels />) },
      { path: 'settings', element: page(<AdminSettings />) },
      { path: 'docs', element: page(<AdminDocs />) },
      { path: '*', element: <Navigate to="/admin" replace /> },
    ],
  },
  { path: '*', element: <Navigate to="/" replace /> },
])
