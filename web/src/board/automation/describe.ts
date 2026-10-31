import type {
  AutomationAction,
  AutomationCondition,
  AutomationRule,
  AutomationTrigger,
  BoardDetail,
  Column,
  Member,
} from '../../api/types'
import { formatDate, PRIORITY_STYLE, isPriority } from '../format'

export interface DescribeContext {
  board: BoardDetail
  members: Member[]
}

const colName = (ctx: DescribeContext, id?: string) => ctx.board.kolom.find((c) => c.id === id)?.judul ?? 'kolom terhapus'
const groupName = (ctx: DescribeContext, id?: string) => ctx.board.grup.find((g) => g.id === id)?.judul ?? 'grup terhapus'
const userName = (ctx: DescribeContext, id?: string) => ctx.members.find((m) => m.user_id === id)?.nama ?? 'pengguna'

function labelName(ctx: DescribeContext, id: string | undefined, colId?: string): string {
  if (id === '') return 'kosong'
  for (const c of ctx.board.kolom) {
    if (c.tipe !== 'status' || (colId && c.id !== colId)) continue
    const l = c.konfigurasi.labels?.find((x) => x.id === id)
    if (l) return l.judul
  }
  return id ?? ''
}

/** Nilai kolom sebagai teks pendek, mis. label status atau nama orang. */
export function valueLabel(ctx: DescribeContext, col: Column | undefined, v: unknown): string {
  if (v === null || v === undefined || v === '') return 'kosong'
  if (!col) return String(v)
  switch (col.tipe) {
    case 'status':
      return labelName(ctx, String(v), col.id)
    case 'priority':
      return isPriority(v) ? PRIORITY_STYLE[v].judul : String(v)
    case 'person':
      return Array.isArray(v) ? (v.length ? v.map((id) => userName(ctx, String(id))).join(', ') : 'kosong') : String(v)
    case 'date':
      return formatDate(String(v))
    case 'progress':
      return `${String(v)}%`
    default:
      return String(v)
  }
}

export function describeTrigger(ctx: DescribeContext, t: AutomationTrigger): string {
  switch (t.jenis) {
    case 'item_dibuat':
      return 'Saat item dibuat'
    case 'status_berubah': {
      const col = t.column_id ? ` ${colName(ctx, t.column_id)}` : ''
      const dari = t.dari !== undefined ? ` dari ${labelName(ctx, t.dari, t.column_id)}` : ''
      const ke = t.ke !== undefined ? ` menjadi ${labelName(ctx, t.ke, t.column_id)}` : ''
      return `Saat status${col} berubah${dari}${ke}`
    }
    case 'penugasan_berubah':
      return `Saat penanggung jawab${t.column_id ? ` (${colName(ctx, t.column_id)})` : ''} berubah`
    case 'tanggal_tiba':
      return `Saat tanggal ${colName(ctx, t.column_id)} tiba (hari ini)`
  }
}

export function describeCondition(ctx: DescribeContext, c: AutomationCondition): string {
  switch (c.jenis) {
    case 'kolom_sama': {
      const col = ctx.board.kolom.find((x) => x.id === c.column_id)
      return `${colName(ctx, c.column_id)} = ${valueLabel(ctx, col, c.nilai)}`
    }
    case 'person_termasuk':
      return `${colName(ctx, c.column_id)} termasuk ${userName(ctx, c.user_id)}`
    case 'di_grup':
      return `item di grup ${groupName(ctx, c.group_id)}`
  }
}

export function describeAction(ctx: DescribeContext, a: AutomationAction): string {
  switch (a.jenis) {
    case 'pindah_grup':
      return `pindahkan ke grup ${groupName(ctx, a.group_id)}`
    case 'ubah_kolom': {
      const col = ctx.board.kolom.find((x) => x.id === a.column_id)
      return `ubah ${colName(ctx, a.column_id)} menjadi ${valueLabel(ctx, col, a.nilai)}`
    }
  }
}

/** Ringkasan satu kalimat, mis. "Saat status menjadi Done, jika ..., maka pindahkan ke grup Selesai". */
export function describeRule(ctx: DescribeContext, r: AutomationRule): string {
  const kondisi = r.kondisi.length ? `, jika ${r.kondisi.map((c) => describeCondition(ctx, c)).join(' dan ')}` : ''
  return `${describeTrigger(ctx, r.pemicu)}${kondisi}, maka ${r.aksi.map((a) => describeAction(ctx, a)).join(', lalu ')}.`
}
