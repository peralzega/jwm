import { useMutation, useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query'
import { useState } from 'react'

import { api, errorMessage } from '../api/client'
import type { Item, Page, ShiftedItem } from '../api/types'
import { dependenciesKey, scheduleKey } from './deps/queries'

export const itemsKey = (boardId: string) => ['items', boardId] as const

const PAGE_SIZE = 1000
// Worker otomasi memeriksa antrean tiap detik.
const AUTOMATION_FOLLOW_UP_MS = 1500

/** Memuat semua item board per halaman 1.000; onProgress menerima jumlah yang sudah dimuat. */
export async function fetchAllItems(boardId: string, onProgress?: (loaded: number) => void): Promise<Item[]> {
  const all: Item[] = []
  let cursor: string | null | undefined = undefined
  do {
    const qs: string = cursor ? `&kursor=${encodeURIComponent(cursor)}` : ''
    const page: Page<Item> = await api<Page<Item>>('GET', `/boards/${boardId}/items?batas=${PAGE_SIZE}${qs}`)
    all.push(...page.items)
    onProgress?.(all.length)
    cursor = page.kursor_berikut
  } while (cursor)
  return all
}

export function useBoardItems(boardId: string, enabled: boolean) {
  const [loaded, setLoaded] = useState(0)
  const query = useQuery({
    queryKey: itemsKey(boardId),
    queryFn: () => fetchAllItems(boardId, setLoaded),
    enabled,
  })
  return { ...query, loaded }
}

function replaceItem(qc: QueryClient, boardId: string, id: string, fn: (it: Item) => Item | null) {
  qc.setQueryData<Item[]>(itemsKey(boardId), (old) => {
    if (!old) return old
    const out: Item[] = []
    for (const it of old) {
      if (it.id !== id) {
        out.push(it)
        continue
      }
      const next = fn(it)
      if (next) out.push(next)
    }
    return out
  })
}

type Snapshot = { before: Item | undefined }

/**
 * Mutasi item yang langsung tampil di cache (optimistic) dan dikembalikan
 * bila server menolak. Galat dilaporkan lewat onError. onShifted menerima
 * successor yang ikut digeser otomatis oleh perubahan timeline utama; nilainya
 * sudah dipasang ke cache.
 */
export function useItemMutations(
  boardId: string,
  onError: (message: string | null) => void,
  onShifted?: (items: ShiftedItem[]) => void,
  /** Board punya aturan otomasi aktif: item dimuat ulang setelah worker sempat berjalan. */
  automation = false,
) {
  const qc = useQueryClient()
  const key = itemsKey(boardId)

  // Aturan otomasi berjalan di server beberapa saat setelah perubahan tersimpan;
  // ambil ulang item itu agar hasilnya (pindah grup, nilai baru) tampil.
  function followUp(id: string) {
    if (!automation) return
    window.setTimeout(() => {
      api<Item>('GET', `/items/${id}`)
        .then((it) => replaceItem(qc, boardId, id, () => it))
        .catch(() => undefined)
    }, AUTOMATION_FOLLOW_UP_MS)
  }

  async function snapshot(id: string): Promise<Snapshot> {
    await qc.cancelQueries({ queryKey: key })
    return { before: qc.getQueryData<Item[]>(key)?.find((it) => it.id === id) }
  }

  function rollback(id: string, ctx: Snapshot | undefined, err: unknown) {
    const before = ctx?.before
    if (before) {
      qc.setQueryData<Item[]>(key, (old) => {
        if (!old) return old
        return old.some((it) => it.id === id) ? old.map((it) => (it.id === id ? before : it)) : [...old, before]
      })
    }
    onError(errorMessage(err))
  }

  function accept(saved: Item) {
    onError(null)
    replaceItem(qc, boardId, saved.id, () => saved)
  }

  const setValue = useMutation({
    mutationFn: (v: { itemId: string; columnId: string; nilai: unknown }) =>
      api<Item>('PUT', `/items/${v.itemId}/values/${v.columnId}`, { nilai: v.nilai }),
    onMutate: async (v) => {
      const ctx = await snapshot(v.itemId)
      replaceItem(qc, boardId, v.itemId, (it) => {
        const nilai = Object.fromEntries(Object.entries(it.nilai).filter(([k]) => k !== v.columnId))
        if (v.nilai !== null) nilai[v.columnId] = v.nilai
        return { ...it, nilai }
      })
      return ctx
    },
    onError: (err, v, ctx) => rollback(v.itemId, ctx, err),
    onSuccess: (saved, v) => {
      const { geser_otomatis: shifted, ...item } = saved
      accept(item)
      for (const s of shifted ?? []) {
        replaceItem(qc, boardId, s.id, (it) => ({
          ...it,
          nilai: { ...it.nilai, [v.columnId]: { mulai: s.mulai, selesai: s.selesai } },
        }))
      }
      if (shifted?.length) onShifted?.(shifted)
      followUp(v.itemId)
      void qc.invalidateQueries({ queryKey: scheduleKey(boardId) })
    },
  })

  const setTitle = useMutation({
    mutationFn: (v: { itemId: string; judul: string }) => api<Item>('PATCH', `/items/${v.itemId}`, { judul: v.judul }),
    onMutate: async (v) => {
      const ctx = await snapshot(v.itemId)
      replaceItem(qc, boardId, v.itemId, (it) => ({ ...it, judul: v.judul }))
      return ctx
    },
    onError: (err, v, ctx) => rollback(v.itemId, ctx, err),
    onSuccess: accept,
  })

  const move = useMutation({
    mutationFn: (v: { itemId: string; groupId: string }) =>
      api<Item>('POST', `/items/${v.itemId}/move`, { group_id: v.groupId }),
    onMutate: async (v) => {
      const ctx = await snapshot(v.itemId)
      // Sementara di akhir grup tujuan; urutan pasti datang dari server.
      replaceItem(qc, boardId, v.itemId, (it) => ({ ...it, group_id: v.groupId, urutan: Number.MAX_SAFE_INTEGER }))
      return ctx
    },
    onError: (err, v, ctx) => rollback(v.itemId, ctx, err),
    onSuccess: accept,
  })

  const remove = useMutation({
    mutationFn: (v: { itemId: string }) => api<undefined>('DELETE', `/items/${v.itemId}`),
    onMutate: async (v) => {
      const ctx = await snapshot(v.itemId)
      replaceItem(qc, boardId, v.itemId, () => null)
      return ctx
    },
    onError: (err, v, ctx) => rollback(v.itemId, ctx, err),
    onSuccess: () => {
      onError(null)
      // Dependensi item terhapus tidak lagi dihitung.
      void qc.invalidateQueries({ queryKey: dependenciesKey(boardId) })
      void qc.invalidateQueries({ queryKey: scheduleKey(boardId) })
    },
  })

  const create = useMutation({
    mutationFn: (v: { groupId: string; judul: string }) =>
      api<Item>('POST', `/boards/${boardId}/items`, { group_id: v.groupId, judul: v.judul }),
    onSuccess: (saved) => {
      onError(null)
      qc.setQueryData<Item[]>(key, (old) => (old ? [...old, saved] : [saved]))
      followUp(saved.id)
    },
    onError: (err) => onError(errorMessage(err)),
  })

  return { setValue, setTitle, move, remove, create }
}

export type ItemMutations = ReturnType<typeof useItemMutations>
