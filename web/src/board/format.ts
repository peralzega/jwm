// Format tampilan dan pemeriksaan masukan nilai sel. Nilai dari API bertipe
// unknown (JSONB per sel), jadi setiap pembaca memeriksa bentuknya.

import type { Priority } from '../api/types'

export interface Timeline {
  mulai: string
  selesai: string
}

export function asString(v: unknown): string | null {
  return typeof v === 'string' ? v : null
}

export function asNumber(v: unknown): number | null {
  return typeof v === 'number' && Number.isFinite(v) ? v : null
}

export function asStringArray(v: unknown): string[] {
  return Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : []
}

export function asTimeline(v: unknown): Timeline | null {
  if (typeof v !== 'object' || v === null) return null
  const { mulai, selesai } = v as Record<string, unknown>
  return typeof mulai === 'string' && typeof selesai === 'string' ? { mulai, selesai } : null
}

const DATE_RE = /^\d{4}-\d{2}-\d{2}$/

/** Tanggal API YYYY-MM-DD; diformat sebagai UTC agar tidak bergeser zona waktu. */
const dateFmt = new Intl.DateTimeFormat('id-ID', { day: 'numeric', month: 'short', year: 'numeric', timeZone: 'UTC' })
const shortDateFmt = new Intl.DateTimeFormat('id-ID', { day: 'numeric', month: 'short', timeZone: 'UTC' })

export function formatDate(iso: string | null, short = false): string {
  if (!iso || !DATE_RE.test(iso)) return ''
  const d = new Date(`${iso}T00:00:00Z`)
  return Number.isNaN(d.getTime()) ? '' : (short ? shortDateFmt : dateFmt).format(d)
}

export function formatTimeline(t: Timeline | null): string {
  if (!t) return ''
  return `${formatDate(t.mulai, true)} – ${formatDate(t.selesai, true)}`
}

const numberFmt = new Intl.NumberFormat('id-ID', { maximumFractionDigits: 6 })

export function formatNumber(n: number | null, satuan?: string): string {
  if (n === null) return ''
  return satuan ? `${numberFmt.format(n)} ${satuan}` : numberFmt.format(n)
}

export type Parsed<T> = { ok: true; value: T } | { ok: false; message: string }

/**
 * Membaca angka dari masukan pengguna. Koma diterima sebagai pemisah desimal
 * (kebiasaan Indonesia); titik ribuan tidak didukung agar tidak ambigu.
 * String kosong berarti kosongkan sel (null).
 */
export function parseNumber(input: string): Parsed<number | null> {
  const s = input.trim().replace(/\s/g, '').replace(',', '.')
  if (s === '') return { ok: true, value: null }
  if (!/^-?\d+(\.\d+)?$/.test(s)) return { ok: false, message: 'Masukkan angka, mis. 12 atau 3,5.' }
  const n = Number(s)
  if (!Number.isFinite(n) || Math.abs(n) > 1e15) return { ok: false, message: 'Angka di luar rentang.' }
  return { ok: true, value: n }
}

/** Progres: bilangan bulat 0 sampai 100; tanda % boleh ikut diketik. */
export function parseProgress(input: string): Parsed<number | null> {
  const s = input.trim().replace(/%$/, '').trim()
  if (s === '') return { ok: true, value: null }
  if (!/^\d{1,3}$/.test(s) || Number(s) > 100) return { ok: false, message: 'Progres harus bilangan bulat 0 sampai 100.' }
  return { ok: true, value: Number(s) }
}

export function isValidDate(s: string): boolean {
  return DATE_RE.test(s) && !Number.isNaN(new Date(`${s}T00:00:00Z`).getTime())
}

/** Timeline dari dua masukan tanggal; keduanya kosong berarti kosongkan sel. */
export function parseTimeline(mulai: string, selesai: string): Parsed<Timeline | null> {
  if (!mulai && !selesai) return { ok: true, value: null }
  if (!isValidDate(mulai) || !isValidDate(selesai)) return { ok: false, message: 'Isi tanggal mulai dan selesai.' }
  if (selesai < mulai) return { ok: false, message: 'Selesai tidak boleh sebelum mulai.' }
  return { ok: true, value: { mulai, selesai } }
}

export const PRIORITY_STYLE: Record<Priority, { judul: string; warna: string }> = {
  rendah: { judul: 'Rendah', warna: 'abu' },
  sedang: { judul: 'Sedang', warna: 'biru' },
  tinggi: { judul: 'Tinggi', warna: 'oranye' },
  kritis: { judul: 'Kritis', warna: 'merah' },
}

export function isPriority(v: unknown): v is Priority {
  return typeof v === 'string' && v in PRIORITY_STYLE
}

/** Inisial dua huruf untuk avatar, mis. "Budi Santoso" → "BS". */
export function initials(nama: string): string {
  const parts = nama.trim().split(/\s+/).filter(Boolean)
  const first = parts[0]?.[0] ?? '?'
  const second = parts.length > 1 ? (parts[parts.length - 1]?.[0] ?? '') : (parts[0]?.[1] ?? '')
  return (first + second).toUpperCase()
}
