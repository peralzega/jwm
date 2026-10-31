// Panah dependensi FS: dari ujung kanan batang predecessor ke awal batang
// successor. Rute berbentuk siku; bila successor mulai sebelum predecessor
// selesai, panah memutar lewat celah antar-baris.

import { barBox, type Span } from './scale'

export interface DependencyLink {
  id: string
  predecessor_id: string
  successor_id: string
}

export interface Arrow {
  id: string
  d: string
  /**
   * Aturan FS dilanggar. Dari backend (memperhitungkan lag dan hari kerja) bila
   * tersedia; untuk batang yang sedang diubah: successor mulai pada atau sebelum
   * predecessor selesai.
   */
  violated: boolean
  /** Rentang vertikal (px) untuk menyaring panah di luar area terlihat. */
  top: number
  bottom: number
}

/** Path SVG dari (x1,y1) ke (x2,y2) dengan jarak siku gap dan tinggi baris rowH. */
export function arrowPath(x1: number, y1: number, x2: number, y2: number, gap: number, rowH: number): string {
  if (x2 - x1 >= 2 * gap) {
    const xm = x1 + gap
    return `M${x1} ${y1}H${xm}V${y2}H${x2}`
  }
  const ym = y1 + (y2 >= y1 ? rowH / 2 : -rowH / 2)
  return `M${x1} ${y1}H${x1 + gap}V${ym}H${x2 - gap}V${y2}H${x2}`
}

/**
 * Panah untuk semua dependensi yang kedua itemnya tampil (punya jadwal dan
 * barisnya tidak tersembunyi). rowIndex memetakan id item ke indeks baris.
 * server.local berisi item yang jadwalnya sedang diubah di klien (draf).
 */
export function buildArrows(
  deps: DependencyLink[],
  spans: ReadonlyMap<string, Span>,
  rowIndex: ReadonlyMap<string, number>,
  geo: { range: Span; dayW: number; rowH: number; gap: number },
  server?: { violated: ReadonlySet<string>; local: ReadonlySet<string> },
): Arrow[] {
  const out: Arrow[] = []
  for (const d of deps) {
    const ps = spans.get(d.predecessor_id)
    const ss = spans.get(d.successor_id)
    const pr = rowIndex.get(d.predecessor_id)
    const sr = rowIndex.get(d.successor_id)
    if (!ps || !ss || pr === undefined || sr === undefined) continue
    const pb = barBox(ps, geo.range, geo.dayW)
    const sb = barBox(ss, geo.range, geo.dayW)
    const y1 = pr * geo.rowH + geo.rowH / 2
    const y2 = sr * geo.rowH + geo.rowH / 2
    out.push({
      id: d.id,
      d: arrowPath(pb.left + pb.width, y1, sb.left, y2, geo.gap, geo.rowH),
      violated:
        server && !server.local.has(d.predecessor_id) && !server.local.has(d.successor_id)
          ? server.violated.has(d.id)
          : ss.start <= ps.end,
      top: Math.min(y1, y2) - geo.rowH,
      bottom: Math.max(y1, y2) + geo.rowH,
    })
  }
  return out
}
