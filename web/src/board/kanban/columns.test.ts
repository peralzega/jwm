import { describe, expect, it } from 'vitest'

import { makeBoard, makeItem } from '../testData'
import { groupByStatus, NO_STATUS, statusColumns, statusValueFor } from './columns'

const board = makeBoard()
const statusCol = board.kolom[0]
if (!statusCol) throw new Error('fixture tanpa kolom status')

describe('groupByStatus', () => {
  const items = [
    makeItem({ id: 'a', judul: 'A', urutan: 2, nilai: { 'c-status': 'done' } }),
    makeItem({ id: 'b', judul: 'B', urutan: 1, nilai: { 'c-status': 'done' } }),
    makeItem({ id: 'c', judul: 'C', group_id: 'g2', urutan: 0, nilai: { 'c-status': 'done' } }),
    makeItem({ id: 'd', judul: 'D' }),
    makeItem({ id: 'e', judul: 'E', nilai: { 'c-status': 'label_terhapus' } }),
    makeItem({ id: 'f', judul: 'F', nilai: { 'c-status': 42 } }),
  ]
  const cols = groupByStatus(statusCol, board.grup, items)

  it('kolom "Tanpa status" pertama, lalu label sesuai urutan konfigurasi', () => {
    expect(cols.map((c) => c.id)).toEqual([NO_STATUS, 'ready_to_start', 'in_progress', 'done'])
    expect(cols.map((c) => c.warna)).toEqual([null, 'abu', 'oranye', 'hijau'])
  })

  it('item tanpa status, dengan label yang sudah tidak ada, atau nilai rusak masuk "Tanpa status"', () => {
    expect(cols[0]?.items.map((i) => i.id)).toEqual(['d', 'e', 'f'])
  })

  it('kartu urut menurut grup lalu urutan item, seperti tabel', () => {
    expect(cols[3]?.items.map((i) => i.id)).toEqual(['b', 'a', 'c'])
  })

  it('kolom tanpa kartu tetap ada', () => {
    expect(cols[1]?.items).toEqual([])
  })
})

describe('statusValueFor dan statusColumns', () => {
  it('melepas di "Tanpa status" mengirim null', () => {
    expect(statusValueFor(NO_STATUS)).toBeNull()
    expect(statusValueFor('done')).toBe('done')
  })

  it('hanya kolom bertipe status, urut menurut urutan', () => {
    const extra = { ...statusCol, id: 'c-status-2', judul: 'Status QA', urutan: 0 }
    expect(statusColumns([...board.kolom, extra]).map((c) => c.id)).toEqual(['c-status-2', 'c-status'])
  })
})
