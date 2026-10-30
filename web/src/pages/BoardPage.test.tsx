import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'

import type { BoardDetail, Group, Item } from '../api/types'
import { meReply, mockApi, renderApp, user } from '../test/utils'
import { groupItems } from './BoardPage'

const groups: Group[] = [
  { id: 'g2', board_id: 'b1', judul: 'Selesai', warna: 'hijau', urutan: 2048 },
  { id: 'g1', board_id: 'b1', judul: 'Tugas', warna: 'biru', urutan: 1024 },
]

function item(over: Partial<Item>): Item {
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
    bisa_ubah: false,
    bisa_hapus: false,
    ...over,
  }
}

const board: BoardDetail = {
  id: 'b1',
  project_id: 'p1',
  nama: 'Board utama',
  urutan: 1024,
  peran_saya: 'member',
  grup: groups,
  kolom: [
    {
      id: 'c-status',
      board_id: 'b1',
      tipe: 'status',
      judul: 'Status',
      urutan: 1024,
      konfigurasi: { labels: [{ id: 'done', judul: 'Done', warna: 'hijau' }] },
    },
    { id: 'c-person', board_id: 'b1', tipe: 'person', judul: 'Penanggung jawab', urutan: 2048, konfigurasi: {} },
  ],
}

describe('groupItems', () => {
  it('mengurutkan grup dan item menurut urutan', () => {
    const out = groupItems(groups, [
      item({ id: 'b', judul: 'B', urutan: 2048 }),
      item({ id: 'a', judul: 'A', urutan: 512 }),
      item({ id: 'c', judul: 'C', group_id: 'g2' }),
    ])
    expect(out.map((g) => g.group.judul)).toEqual(['Tugas', 'Selesai'])
    expect(out[0]?.items.map((i) => i.judul)).toEqual(['A', 'B'])
    expect(out[1]?.items.map((i) => i.judul)).toEqual(['C'])
  })
})

describe('halaman board', () => {
  it('mengikuti hak dari server: item orang lain hanya-baca, item sendiri bisa diubah dan dihapus', async () => {
    const calls = mockApi({
      'GET /auth/me': meReply(user()),
      'GET /boards/b1': { status: 200, body: board },
      'GET /boards/b1/items': {
        status: 200,
        body: {
          items: [
            item({ id: 'mine', judul: 'Milik saya', bisa_ubah: true, bisa_hapus: true, nilai: { 'c-person': ['u-1'] } }),
            item({ id: 'other', judul: 'Milik orang lain', urutan: 2048 }),
          ],
          kursor_berikut: null,
        },
      },
      'GET /projects/p1/members': { status: 200, body: { items: [{ user_id: 'u-1', nama: 'Budi' }] } },
      'PUT /items/mine/values/c-status': { status: 200, body: {} },
      'POST /items/mine/move': { status: 200, body: {} },
    })
    renderApp('/board/b1')

    const tugas = await screen.findByRole('region', { name: 'Grup Tugas' })
    expect(await within(tugas).findByLabelText('Judul item Milik saya')).toBeInTheDocument()
    expect(within(tugas).getByRole('button', { name: 'Hapus item Milik saya' })).toBeInTheDocument()
    expect(within(tugas).getByText('Budi')).toBeInTheDocument()

    expect(within(tugas).queryByLabelText('Judul item Milik orang lain')).not.toBeInTheDocument()
    expect(within(tugas).getByText('Milik orang lain')).toBeInTheDocument()
    expect(within(tugas).getByLabelText('Status item Milik orang lain')).toBeDisabled()
    expect(within(tugas).queryByRole('button', { name: 'Hapus item Milik orang lain' })).not.toBeInTheDocument()

    expect(within(tugas).getByLabelText('Item baru di Tugas')).toBeInTheDocument()
    expect(screen.queryByRole('form', { name: 'Tambah grup' })).not.toBeInTheDocument()

    const u = userEvent.setup()
    await u.selectOptions(within(tugas).getByLabelText('Status item Milik saya'), 'done')
    await waitFor(() =>
      expect(calls.find((c) => c.path === '/items/mine/values/c-status')?.body).toEqual({ nilai: 'done' }),
    )
    await u.selectOptions(within(tugas).getByLabelText('Pindah grup item Milik saya'), 'g2')
    await waitFor(() => expect(calls.find((c) => c.path === '/items/mine/move')?.body).toEqual({ group_id: 'g2' }))
  })

  it('board yang tidak dapat diakses menampilkan halaman tidak ditemukan', async () => {
    mockApi({ 'GET /auth/me': meReply(user()) })
    renderApp('/board/rahasia')
    expect(await screen.findByRole('heading', { name: 'Halaman tidak ditemukan' })).toBeInTheDocument()
  })
})
