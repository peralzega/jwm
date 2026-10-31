import { useQueryClient, type QueryKey } from '@tanstack/react-query'
import { useEffect, useState } from 'react'

export type LiveState = 'menyambung' | 'langsung' | 'terputus' | 'tidak_didukung'

/**
 * Berlangganan GET /api/v1/stream (SSE) untuk satu proyek atau semua proyek
 * yang terlihat (projectId null), lalu memuat ulang query terkait saat ada
 * event "berubah". Event tidak berisi data; data diambil ulang lewat API biasa.
 * EventSource menyambung ulang sendiri setelah server menutup (umur maksimal).
 */
export function useLiveUpdates(projectId: string | null, keys: QueryKey[]): LiveState {
  const qc = useQueryClient()
  const supported = typeof EventSource !== 'undefined'
  const [state, setState] = useState<LiveState>(supported ? 'menyambung' : 'tidak_didukung')
  const keyText = JSON.stringify(keys)

  useEffect(() => {
    if (!supported) return
    const url = projectId ? `/api/v1/stream?project=${encodeURIComponent(projectId)}` : '/api/v1/stream'
    const es = new EventSource(url)
    const parsed = JSON.parse(keyText) as QueryKey[]
    let timer: number | undefined
    es.onopen = () => setState('langsung')
    es.onerror = () => setState(es.readyState === EventSource.CLOSED ? 'terputus' : 'menyambung')
    es.addEventListener('berubah', () => {
      // Beberapa event berdekatan digabung menjadi satu kali muat ulang.
      window.clearTimeout(timer)
      timer = window.setTimeout(() => {
        for (const k of parsed) void qc.invalidateQueries({ queryKey: k })
      }, 200)
    })
    return () => {
      window.clearTimeout(timer)
      es.close()
    }
  }, [projectId, keyText, qc, supported])

  return state
}
