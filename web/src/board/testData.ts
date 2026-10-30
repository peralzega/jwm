// Data uji bersama untuk tes tabel board.

import type { BoardDetail, Item, Member } from '../api/types'

export function makeItem(over: Partial<Item> = {}): Item {
  return {
    id: 'i',
    board_id: 'b1',
    group_id: 'g1',
    judul: 'Item',
    urutan: 1024,
    dibuat_oleh: 'u-1',
    dibuat_pada: '2026-10-29T00:00:00Z',
    diubah_pada: '2026-10-29T00:00:00Z',
    nilai: {},
    bisa_ubah: true,
    bisa_hapus: true,
    ...over,
  }
}

/** Board dengan semua tipe kolom; urutan kolom = urutan di tabel (kolom 1..9). */
export function makeBoard(over: Partial<BoardDetail> = {}): BoardDetail {
  const col = (id: string, tipe: BoardDetail['kolom'][number]['tipe'], judul: string, urutan: number, konfigurasi = {}) => ({
    id,
    board_id: 'b1',
    tipe,
    judul,
    urutan,
    konfigurasi,
  })
  return {
    id: 'b1',
    project_id: 'p1',
    nama: 'Board utama',
    urutan: 1024,
    peran_saya: 'member',
    grup: [
      { id: 'g1', board_id: 'b1', judul: 'Tugas', warna: 'biru', urutan: 1024 },
      { id: 'g2', board_id: 'b1', judul: 'Selesai', warna: 'hijau', urutan: 2048 },
    ],
    kolom: [
      col('c-status', 'status', 'Status', 1, {
        labels: [
          { id: 'ready_to_start', judul: 'Ready to start', warna: 'abu' },
          { id: 'in_progress', judul: 'In progress', warna: 'oranye' },
          { id: 'done', judul: 'Done', warna: 'hijau' },
        ],
      }),
      col('c-person', 'person', 'Penanggung jawab', 2),
      col('c-priority', 'priority', 'Prioritas', 3),
      col('c-date', 'date', 'Tenggat', 4),
      col('c-timeline', 'timeline', 'Timeline', 5),
      col('c-number', 'number', 'Estimasi', 6, { satuan: 'jam' }),
      col('c-progress', 'progress', 'Progres', 7),
      col('c-text', 'text', 'Catatan', 8),
      col('c-dep', 'dependency', 'Dependensi', 9),
    ],
    ...over,
  }
}

export const COL = {
  title: 0,
  status: 1,
  person: 2,
  priority: 3,
  date: 4,
  timeline: 5,
  number: 6,
  progress: 7,
  text: 8,
  dependency: 9,
  actions: 10,
} as const

export function makeMember(over: Partial<Member>): Member {
  return {
    user_id: 'u-1',
    nama: 'Budi',
    email: 'budi@jiep.co.id',
    peran_global: 'member',
    aktif: true,
    peran_proyek: 'member',
    peran_efektif: 'member',
    ditambahkan_pada: '2026-10-01T00:00:00Z',
    ...over,
  }
}
