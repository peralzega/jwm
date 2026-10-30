import { describe, expect, it } from 'vitest'

import type { Group, Item } from '../api/types'
import { buildRows, countItems } from './rows'
import { makeItem } from './testData'

const groups: Group[] = [
  { id: 'g2', board_id: 'b1', judul: 'Selesai', warna: 'hijau', urutan: 2048 },
  { id: 'g1', board_id: 'b1', judul: 'Tugas', warna: 'biru', urutan: 1024 },
  { id: 'g3', board_id: 'b1', judul: 'Kosong', warna: 'abu', urutan: 4096 },
]

const items: Item[] = [
  makeItem({ id: 'b', judul: 'B', urutan: 2048 }),
  makeItem({ id: 'a', judul: 'A', urutan: 512 }),
  makeItem({ id: 'c', judul: 'C', group_id: 'g2' }),
  makeItem({ id: 'x', judul: 'Yatim', group_id: 'tidak-ada' }),
]

const shape = (rows: ReturnType<typeof buildRows>) =>
  rows.map((r) => (r.kind === 'group' ? `[${r.group.judul}:${r.count}${r.collapsed ? '+' : ''}]` : r.kind === 'item' ? r.item.judul : `+${r.group.id}`))

describe('buildRows', () => {
  const cases: { name: string; collapsed: string[]; canCreate: boolean; want: string[] }[] = [
    {
      name: 'grup dan item urut menurut urutan, baris tambah di akhir grup',
      collapsed: [],
      canCreate: true,
      want: ['[Tugas:2]', 'A', 'B', '+g1', '[Selesai:1]', 'C', '+g2', '[Kosong:0]', '+g3'],
    },
    {
      name: 'tanpa hak membuat: tidak ada baris tambah',
      collapsed: [],
      canCreate: false,
      want: ['[Tugas:2]', 'A', 'B', '[Selesai:1]', 'C', '[Kosong:0]'],
    },
    {
      name: 'grup dilipat hanya menampilkan header dengan jumlah item',
      collapsed: ['g1'],
      canCreate: true,
      want: ['[Tugas:2+]', '[Selesai:1]', 'C', '+g2', '[Kosong:0]', '+g3'],
    },
  ]
  for (const tc of cases) {
    it(tc.name, () => {
      expect(shape(buildRows(groups, items, new Set(tc.collapsed), tc.canCreate))).toEqual(tc.want)
    })
  }

  it('urutan sama diurutkan menurut id agar stabil', () => {
    const rows = buildRows(groups, [makeItem({ id: 'z', judul: 'Z' }), makeItem({ id: 'm', judul: 'M' })], new Set(), false)
    expect(shape(rows).slice(0, 3)).toEqual(['[Tugas:2]', 'M', 'Z'])
  })

  it('countItems menghitung baris item saja', () => {
    expect(countItems(buildRows(groups, items, new Set(), true))).toBe(3)
  })
})
