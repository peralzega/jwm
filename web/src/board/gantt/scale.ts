// Skala waktu Gantt. Tanggal API berbentuk YYYY-MM-DD dan dihitung sebagai
// nomor hari UTC agar tidak bergeser oleh zona waktu atau pergantian jam musim.

export type GanttScale = 'hari' | 'minggu' | 'bulan'

export const SCALES: { id: GanttScale; label: string }[] = [
  { id: 'hari', label: 'Hari' },
  { id: 'minggu', label: 'Minggu' },
  { id: 'bulan', label: 'Bulan' },
]

export function parseScale(v: string | null): GanttScale {
  return v === 'hari' || v === 'bulan' ? v : 'minggu'
}

const DAY_MS = 86_400_000

/** Nomor hari sejak 1970-01-01 (UTC) dari YYYY-MM-DD. */
export function dayNum(iso: string): number {
  return Math.floor(Date.parse(`${iso}T00:00:00Z`) / DAY_MS)
}

export function isoOf(day: number): string {
  return new Date(day * DAY_MS).toISOString().slice(0, 10)
}

/** Hari ini menurut kalender lokal pengguna. */
export function todayNum(now = new Date()): number {
  return Math.floor(Date.UTC(now.getFullYear(), now.getMonth(), now.getDate()) / DAY_MS)
}

/** 0 = Minggu … 6 = Sabtu. */
export function weekday(day: number): number {
  return new Date(day * DAY_MS).getUTCDay()
}

function startOfWeek(day: number): number {
  // Minggu dimulai Senin.
  return day - ((weekday(day) + 6) % 7)
}

function startOfMonth(day: number): number {
  const d = new Date(day * DAY_MS)
  return Math.floor(Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), 1) / DAY_MS)
}

function addMonths(day: number, n: number): number {
  const d = new Date(day * DAY_MS)
  return Math.floor(Date.UTC(d.getUTCFullYear(), d.getUTCMonth() + n, 1) / DAY_MS)
}

export interface Span {
  start: number
  end: number
}

// Ruang kosong di kiri dan kanan rentang data, dalam hari.
const PAD_BEFORE = 7
const PAD_AFTER = 21
// Rentang minimum agar board kosong tetap menampilkan sumbu waktu yang berguna.
const MIN_DAYS = 56

/**
 * Rentang sumbu waktu: semua jadwal ditambah hari ini, diberi ruang, lalu
 * dibulatkan ke awal minggu atau bulan sesuai skala. end bersifat eksklusif.
 */
export function chartRange(spans: Span[], today: number, scale: GanttScale): Span {
  let lo = today
  let hi = today
  for (const s of spans) {
    lo = Math.min(lo, s.start)
    hi = Math.max(hi, s.end)
  }
  lo -= PAD_BEFORE
  hi = Math.max(hi + PAD_AFTER, lo + MIN_DAYS)
  if (scale === 'bulan') return { start: startOfMonth(lo), end: addMonths(hi, 1) }
  const start = startOfWeek(lo)
  return { start, end: startOfWeek(hi) + 7 }
}

/** Posisi x (px) awal hari day. */
export function xOf(day: number, range: Span, dayW: number): number {
  return (day - range.start) * dayW
}

/** Geometri batang untuk jadwal inklusif [mulai, selesai]. */
export function barBox(span: Span, range: Span, dayW: number): { left: number; width: number } {
  return { left: xOf(span.start, range, dayW), width: (span.end - span.start + 1) * dayW }
}

/** Pergeseran px dibulatkan ke hari terdekat, simetris ke kiri dan kanan. */
export function daysFromPx(dx: number, dayW: number): number {
  return Math.sign(dx) * Math.round(Math.abs(dx) / dayW) || 0
}

export type DragMode = 'geser' | 'mulai' | 'selesai'

/**
 * Jadwal baru setelah diseret delta hari. Mengubah mulai atau selesai tidak
 * boleh membuat batang lebih pendek dari satu hari.
 */
export function applyDrag(span: Span, mode: DragMode, delta: number): Span {
  switch (mode) {
    case 'geser':
      return { start: span.start + delta, end: span.end + delta }
    case 'mulai':
      return { start: Math.min(span.start + delta, span.end), end: span.end }
    case 'selesai':
      return { start: span.start, end: Math.max(span.end + delta, span.start) }
  }
}

export interface Tick {
  key: string
  left: number
  width: number
  label: string
  /** Label lengkap untuk title (mis. tanggal penuh). */
  title: string
}

const fmt = (o: Intl.DateTimeFormatOptions) => new Intl.DateTimeFormat('id-ID', { ...o, timeZone: 'UTC' })
const dayFmt = fmt({ day: 'numeric' })
const fullFmt = fmt({ weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' })
const weekFmt = fmt({ day: 'numeric', month: 'short' })
const monthFmt = fmt({ month: 'long', year: 'numeric' })
const monthShortFmt = fmt({ month: 'short' })
const yearFmt = fmt({ year: 'numeric' })

const dateOf = (day: number) => new Date(day * DAY_MS)

function months(range: Span, dayW: number, label: Intl.DateTimeFormat): Tick[] {
  const out: Tick[] = []
  for (let m = startOfMonth(range.start); m < range.end; m = addMonths(m, 1)) {
    const lo = Math.max(m, range.start)
    const hi = Math.min(addMonths(m, 1), range.end)
    out.push({ key: `m${m}`, left: xOf(lo, range, dayW), width: (hi - lo) * dayW, label: label.format(dateOf(m)), title: monthFmt.format(dateOf(m)) })
  }
  return out
}

/** Dua baris header: baris atas (konteks) dan baris bawah (satuan skala). */
export function headerTicks(range: Span, scale: GanttScale, dayW: number): { top: Tick[]; bottom: Tick[] } {
  if (scale === 'bulan') {
    const top: Tick[] = []
    for (let m = range.start; m < range.end; ) {
      const y = dateOf(m).getUTCFullYear()
      const next = Math.min(Math.floor(Date.UTC(y + 1, 0, 1) / DAY_MS), range.end)
      top.push({ key: `y${y}`, left: xOf(m, range, dayW), width: (next - m) * dayW, label: String(y), title: yearFmt.format(dateOf(m)) })
      m = next
    }
    return { top, bottom: months(range, dayW, monthShortFmt) }
  }
  const top = months(range, dayW, monthFmt)
  const bottom: Tick[] = []
  if (scale === 'hari') {
    for (let d = range.start; d < range.end; d++) {
      bottom.push({ key: `d${d}`, left: xOf(d, range, dayW), width: dayW, label: dayFmt.format(dateOf(d)), title: fullFmt.format(dateOf(d)) })
    }
  } else {
    for (let w = range.start; w < range.end; w += 7) {
      bottom.push({ key: `w${w}`, left: xOf(w, range, dayW), width: 7 * dayW, label: weekFmt.format(dateOf(w)), title: `Minggu mulai ${fullFmt.format(dateOf(w))}` })
    }
  }
  return { top, bottom }
}

const spanFmt = fmt({ day: 'numeric', month: 'short', year: 'numeric' })

/** Teks jadwal untuk label aksesibilitas, mis. "3 Mar 2026 – 7 Mar 2026, 5 hari". */
export function describeSpan(span: Span): string {
  const days = span.end - span.start + 1
  return `${spanFmt.format(dateOf(span.start))} – ${spanFmt.format(dateOf(span.end))}, ${days} hari`
}
