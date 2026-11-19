import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from 'react-router'
import { Toaster } from '@/components/ui/sonner'
import { TooltipProvider } from '@/components/ui/tooltip'
import { ConfirmProvider } from '@/components/confirm'
import { AuthProvider } from '@/lib/auth'
import { ApiError } from '@/lib/api'
import { SiteBranding } from '@/components/site-brand'
import { router } from './router'
import './index.gen.css'

// 外壳 HTML 由 webfront 生成，不保证带 .dark；主题只有暗色一套，这里补上。
document.documentElement.classList.add('dark')

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 15_000,
      refetchOnWindowFocus: false,
      retry: (count, err) => !(err instanceof ApiError && err.status >= 400 && err.status < 500) && count < 2,
    },
  },
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <SiteBranding />
      <AuthProvider>
        <TooltipProvider delayDuration={300}>
          <ConfirmProvider>
            <RouterProvider router={router} />
          </ConfirmProvider>
        </TooltipProvider>
        <Toaster position="bottom-right" toastOptions={{ className: 'text-[13px]' }} />
      </AuthProvider>
    </QueryClientProvider>
  </StrictMode>,
)
