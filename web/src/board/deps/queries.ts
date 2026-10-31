import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api, errorMessage } from '../../api/client'
import type { Dependency, ScheduleView } from '../../api/types'

// Dependensi dan hasil CPM dibagi oleh tabel dan Gantt lewat cache TanStack Query.

export const dependenciesKey = (boardId: string) => ['dependencies', boardId] as const
/** Semua hasil CPM (mis. setelah kalender kerja berubah). */
export const ALL_SCHEDULES_KEY = ['schedule'] as const
/** Tanpa columnId: semua jadwal board (untuk invalidasi). */
export const scheduleKey = (boardId: string, columnId?: string) =>
  columnId ? (['schedule', boardId, columnId] as const) : (['schedule', boardId] as const)

const NO_DEPS: Dependency[] = []

export function useDependencies(boardId: string, enabled = true) {
  const q = useQuery({
    queryKey: dependenciesKey(boardId),
    enabled,
    queryFn: () => api<{ items: Dependency[] }>('GET', `/boards/${boardId}/dependencies`).then((r) => r.items),
  })
  return { ...q, deps: q.data ?? NO_DEPS }
}

export function useSchedule(boardId: string, columnId: string) {
  return useQuery({
    queryKey: scheduleKey(boardId, columnId),
    queryFn: () =>
      api<ScheduleView>('GET', `/boards/${boardId}/schedule?timeline=${encodeURIComponent(columnId)}`),
  })
}

/** Tambah dan hapus dependensi; jadwal board dihitung ulang setelahnya. */
export function useDependencyMutations(boardId: string) {
  const qc = useQueryClient()
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: dependenciesKey(boardId) })
    void qc.invalidateQueries({ queryKey: scheduleKey(boardId) })
  }
  const create = useMutation({
    mutationFn: (v: { predecessorId: string; successorId: string; lagHari: number }) =>
      api<Dependency>('POST', `/boards/${boardId}/dependencies`, {
        predecessor_id: v.predecessorId,
        successor_id: v.successorId,
        lag_hari: v.lagHari,
      }),
    onSuccess: refresh,
  })
  const remove = useMutation({
    mutationFn: (v: { id: string }) => api<undefined>('DELETE', `/dependencies/${v.id}`),
    onSuccess: refresh,
  })
  const error = create.error ?? remove.error
  return { create, remove, errorText: error ? errorMessage(error) : null }
}
