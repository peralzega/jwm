import type { Column, Group, Item, StatusLabel } from '../../api/types'
import { asString } from '../format'

/** Id kolom Kanban untuk item tanpa status (bukan id label yang sah di server). */
export const NO_STATUS = '__tanpa_status__'

export interface KanbanColumnData {
  /** Id label status, atau NO_STATUS. */
  id: string
  judul: string
  warna: string | null
  items: Item[]
}

/**
 * Mengelompokkan item per label status kolom `statusCol`. Kolom "Tanpa status"
 * selalu pertama; sisanya mengikuti urutan label. Item dengan label yang tidak
 * lagi ada di konfigurasi ikut "Tanpa status". Kartu urut seperti tabel:
 * urutan grup, lalu urutan item.
 */
export function groupByStatus(statusCol: Column, groups: Group[], items: Item[]): KanbanColumnData[] {
  const labels: StatusLabel[] = statusCol.konfigurasi.labels ?? []
  const cols: KanbanColumnData[] = [
    { id: NO_STATUS, judul: 'Tanpa status', warna: null, items: [] },
    ...labels.map((l) => ({ id: l.id, judul: l.judul, warna: l.warna, items: [] as Item[] })),
  ]
  const byId = new Map(cols.map((c) => [c.id, c]))
  const groupOrder = new Map(groups.map((g) => [g.id, g.urutan]))

  const sorted = [...items].sort(
    (a, b) =>
      (groupOrder.get(a.group_id) ?? Infinity) - (groupOrder.get(b.group_id) ?? Infinity) ||
      a.urutan - b.urutan ||
      (a.id < b.id ? -1 : a.id > b.id ? 1 : 0),
  )
  for (const it of sorted) {
    const label = asString(it.nilai[statusCol.id])
    const col = (label !== null && byId.get(label)) || byId.get(NO_STATUS)
    col?.items.push(it)
  }
  return cols
}

/** Nilai yang dikirim ke API saat kartu dilepas di kolom `columnId`. */
export function statusValueFor(columnId: string): string | null {
  return columnId === NO_STATUS ? null : columnId
}

/** Kolom status yang dapat dipakai untuk Kanban, urut menurut urutan kolom. */
export function statusColumns(kolom: Column[]): Column[] {
  return kolom.filter((c) => c.tipe === 'status').sort((a, b) => a.urutan - b.urutan)
}
