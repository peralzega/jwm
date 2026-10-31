import { useQuery } from '@tanstack/react-query'

import { api } from '../api/client'
import type { Portfolio } from '../api/types'

export const PORTFOLIOS_KEY = ['portfolios'] as const
/** Dasbor portofolio; semua varian filter di bawah kunci ini. */
export const DASHBOARD_KEY = ['portfolio-dashboard'] as const

export function usePortfolios() {
  return useQuery({
    queryKey: PORTFOLIOS_KEY,
    queryFn: () => api<{ items: Portfolio[] }>('GET', '/portfolios').then((r) => r.items),
  })
}
