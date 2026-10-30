import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api, ApiError } from '../api/client'
import type { SessionResponse, SessionUser } from '../api/types'

export const ME_KEY = ['me'] as const

/** Pengguna saat ini; null bila belum login. */
export function useMe() {
  return useQuery({
    queryKey: ME_KEY,
    queryFn: async (): Promise<SessionUser | null> => {
      try {
        const r = await api<SessionResponse>('GET', '/auth/me')
        return r.user
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) return null
        throw err
      }
    },
    staleTime: 60_000,
    retry: false,
  })
}

export function useLogin() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (v: { email: string; password: string }) => api<SessionResponse>('POST', '/auth/login', v),
    onSuccess: (r) => {
      qc.clear()
      qc.setQueryData(ME_KEY, r.user)
    },
  })
}

export function useChangePassword() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (v: { sandi_lama: string; sandi_baru: string }) =>
      api<SessionResponse>('POST', '/auth/password', v),
    onSuccess: (r) => qc.setQueryData(ME_KEY, r.user),
  })
}

export function useLogout() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => api<undefined>('POST', '/auth/logout'),
    onSettled: () => {
      qc.clear()
      qc.setQueryData(ME_KEY, null)
    },
  })
}
