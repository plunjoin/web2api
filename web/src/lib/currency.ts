import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import type { PublicConfig } from '@/lib/types'

/** 平台展示用汇率（人民币兑美元）；与公开配置共用查询缓存。 */
export function useCNYPerUSD() {
  const { data } = useQuery({
    queryKey: ['public-config'],
    queryFn: () => api<PublicConfig>('/api/public/config'),
    staleTime: 60_000,
  })
  return data?.cny_per_usd || 7.2
}
