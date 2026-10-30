import type { Group, Item } from '../api/types'

/** Satu baris pada tabel virtual: header grup, item, atau baris tambah item. */
export type GridRow =
  | { kind: 'group'; key: string; group: Group; count: number; collapsed: boolean }
  | { kind: 'item'; key: string; item: Item; group: Group }
  | { kind: 'add'; key: string; group: Group }

const byOrder = (a: { urutan: number; id: string }, b: { urutan: number; id: string }) =>
  a.urutan - b.urutan || (a.id < b.id ? -1 : a.id > b.id ? 1 : 0)

/**
 * Menyusun baris datar: tiap grup (urut menurut urutan) diikuti itemnya (urut
 * menurut urutan) dan, bila boleh, baris tambah item. Grup yang dilipat hanya
 * menampilkan headernya. Item dengan grup yang tidak dikenal diabaikan.
 */
export function buildRows(groups: Group[], items: Item[], collapsed: ReadonlySet<string>, canCreate: boolean): GridRow[] {
  const buckets = new Map<string, Item[]>(groups.map((g) => [g.id, []]))
  for (const it of items) buckets.get(it.group_id)?.push(it)

  const rows: GridRow[] = []
  for (const group of [...groups].sort(byOrder)) {
    const list = (buckets.get(group.id) ?? []).sort(byOrder)
    const isCollapsed = collapsed.has(group.id)
    rows.push({ kind: 'group', key: `g:${group.id}`, group, count: list.length, collapsed: isCollapsed })
    if (isCollapsed) continue
    for (const item of list) rows.push({ kind: 'item', key: item.id, item, group })
    if (canCreate) rows.push({ kind: 'add', key: `a:${group.id}`, group })
  }
  return rows
}

/** Jumlah item pada baris; dipakai untuk label aksesibilitas. */
export function countItems(rows: GridRow[]): number {
  let n = 0
  for (const r of rows) if (r.kind === 'item') n++
  return n
}
