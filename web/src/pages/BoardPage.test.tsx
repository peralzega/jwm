import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { BoardDetail, Dependency, Item } from '../api/types'
import { COL, makeBoard, makeItem, makeMember } from '../board/testData'
import { meReply, mockApi, renderApp, user, type Call } from '../test/utils'

// jsdom tidak punya tata letak: beri area gulir tabel ukuran nyata agar
// virtualisasi menghitung baris yang terlihat, dan scrollTo yang memicu scroll.
const VIEW_H = 640
const originals = {
  clientHeight: Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'clientHeight'),
  clientWidth: Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'clientWidth'),
  scrollTo: Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'scrollTo'),
}

beforeEach(() => {
  Object.defineProperty(HTMLElement.prototype, 'clientHeight', {
    configurable: true,
    get(this: HTMLElement) {
      return this.dataset.testid === 'board-scroll' ? VIEW_H : 0
    },
  })
  Object.defineProperty(HTMLElement.prototype, 'clientWidth', {
    configurable: true,
    get(this: HTMLElement) {
      return this.dataset.testid === 'board-scroll' ? 1200 : 0
    },
  })
  Object.defineProperty(HTMLElement.prototype, 'scrollTo', {
    configurable: true,
    value(this: HTMLElement, opts: ScrollToOptions) {
      if (opts.top !== undefined) this.scrollTop = opts.top
      if (opts.left !== undefined) this.scrollLeft = opts.left
      this.dispatchEvent(new Event('scroll'))
    },
  })
  document.documentElement.dataset.density = 'compact'
})

afterEach(() => {
  for (const [k, d] of Object.entries(originals)) {
    if (d) Object.defineProperty(HTMLElement.prototype, k, d)
    else Reflect.deleteProperty(HTMLElement.prototype, k)
  }
})

const members = [makeMember({ user_id: 'u-1', nama: 'Budi' }), makeMember({ user_id: 'u-2', nama: 'Sari Dewi' })]

/** Item tersimpan di "server" tiruan; PUT/PATCH/POST mengembalikan item terbaru. */
function setup(
  items: Item[],
  opts: { board?: BoardDetail; deps?: Dependency[]; reject?: (c: Call) => { status: number; body: unknown } | null } = {},
) {
  const store = new Map(items.map((it) => [it.id, it]))
  const board = opts.board ?? makeBoard()
  const save = (c: Call, fn: (it: Item, body: Record<string, unknown>) => Item) => {
    const rejected = opts.reject?.(c)
    if (rejected) return rejected
    const id = c.path.split('/')[2] ?? ''
    const cur = store.get(id)
    if (!cur) return { status: 404, body: { error: 'not_found' } }
    const next = fn(cur, c.body as Record<string, unknown>)
    store.set(id, next)
    return { status: 200, body: next }
  }
  const handlers: Parameters<typeof mockApi>[0] = {
    'GET /auth/me': meReply(user()),
    'GET /boards/b1': { status: 200, body: board },
    'GET /boards/b1/items': { status: 200, body: { items, kursor_berikut: null } },
    'GET /projects/p1/members': { status: 200, body: { items: members } },
    'GET /boards/b1/dependencies': { status: 200, body: { items: opts.deps ?? [] } },
  }
  for (const it of items) {
    handlers[`PATCH /items/${it.id}`] = (c) => save(c, (cur, b) => ({ ...cur, judul: String(b.judul) }))
    handlers[`POST /items/${it.id}/move`] = (c) => save(c, (cur, b) => ({ ...cur, group_id: String(b.group_id), urutan: 99999 }))
    handlers[`DELETE /items/${it.id}`] = { status: 204 }
    for (const col of board.kolom) {
      handlers[`PUT /items/${it.id}/values/${col.id}`] = (c) =>
        save(c, (cur, b) => {
          const nilai = { ...cur.nilai }
          if (b.nilai === null) Reflect.deleteProperty(nilai, col.id)
          else nilai[col.id] = b.nilai
          return { ...cur, nilai }
        })
    }
  }
  handlers['POST /boards/b1/items'] = (c) => {
    const b = c.body as { group_id: string; judul: string }
    return { status: 201, body: makeItem({ id: 'baru', group_id: b.group_id, judul: b.judul, urutan: 99999 }) }
  }
  const calls = mockApi(handlers)
  renderApp('/board/b1')
  return { calls, writes: () => calls.filter((c) => c.method !== 'GET') }
}

async function rowOf(judul: string): Promise<HTMLElement> {
  const text = await screen.findByText(judul, { selector: '[role=gridcell] span' })
  const row = text.closest<HTMLElement>('[role=row]')
  if (!row) throw new Error(`baris ${judul} tidak ditemukan`)
  return row
}

async function cellOf(judul: string, c: number): Promise<HTMLElement> {
  const cell = within(await rowOf(judul)).getAllByRole('gridcell')[c]
  if (!cell) throw new Error(`sel ${c} tidak ditemukan`)
  return cell
}

describe('tabel board', () => {
  it('virtualisasi: 5.000 item hanya merender baris yang terlihat, dan berganti saat digulir', async () => {
    const many = Array.from({ length: 5000 }, (_, i) =>
      makeItem({ id: `i${i}`, judul: `Tugas ${i}`, urutan: i + 1, group_id: i < 2500 ? 'g1' : 'g2' }),
    )
    setup(many)
    const grid = await screen.findByRole('grid', { name: 'Tabel Board utama, 5000 item' })
    expect(grid).toHaveAttribute('aria-rowcount', String(5000 + 2 * 2 + 1))

    const rendered = () => within(grid).getAllByRole('row').length
    expect(rendered()).toBeGreaterThan(10)
    expect(rendered()).toBeLessThan(60)
    expect(screen.getByText('Tugas 0')).toBeInTheDocument()
    expect(screen.queryByText('Tugas 3000')).not.toBeInTheDocument()

    act(() => {
      grid.scrollTop = 3000 * 32
      grid.dispatchEvent(new Event('scroll'))
    })
    expect(await screen.findByText('Tugas 2998')).toBeInTheDocument()
    expect(screen.queryByText('Tugas 0')).not.toBeInTheDocument()
    expect(rendered()).toBeLessThan(60)
  })

  it('header dan kolom judul menempel', async () => {
    setup([makeItem({ id: 'a', judul: 'Alpha' })])
    const header = (await screen.findAllByRole('columnheader'))[0]
    expect(header?.parentElement).toHaveClass('sticky', 'top-0')
    expect(header).toHaveClass('sticky', 'left-0')
    expect(await cellOf('Alpha', COL.title)).toHaveClass('sticky', 'left-0')
    expect(screen.getAllByRole('columnheader').map((h) => h.textContent)).toEqual([
      'Item',
      'Status',
      'Penanggung jawab',
      'Prioritas',
      'Tenggat',
      'Timeline',
      'Estimasi',
      'Progres',
      'Catatan',
      'Dependensi',
      '',
    ])
  })

  it('menampilkan nilai per tipe; status dan prioritas sebagai lencana berwarna', async () => {
    setup([
      makeItem({
        id: 'a',
        judul: 'Alpha',
        nilai: {
          'c-status': 'in_progress',
          'c-person': ['u-2'],
          'c-priority': 'kritis',
          'c-date': '2026-11-03',
          'c-timeline': { mulai: '2026-11-01', selesai: '2026-11-10' },
          'c-number': 7.5,
          'c-progress': 40,
          'c-text': 'catatan',
        },
      }),
    ])
    const status = within(await cellOf('Alpha', COL.status)).getByText('In progress')
    expect(status).toHaveClass('tag-oranye', 'tag-fill')
    expect(within(await cellOf('Alpha', COL.priority)).getByText('Kritis')).toHaveClass('tag-merah', 'tag-fill')
    expect(await cellOf('Alpha', COL.person)).toHaveTextContent('Sari Dewi')
    expect(await cellOf('Alpha', COL.date)).toHaveTextContent(/3 Nov/)
    expect(await cellOf('Alpha', COL.timeline)).toHaveTextContent(/1 Nov – 10 Nov/)
    expect(await cellOf('Alpha', COL.number)).toHaveTextContent('7,5 jam')
    expect(await cellOf('Alpha', COL.progress)).toHaveTextContent('40%')
    expect(await cellOf('Alpha', COL.text)).toHaveTextContent('catatan')
  })

  it('edit sel inline: judul, teks, angka, progres, tanggal', async () => {
    const { writes } = setup([makeItem({ id: 'a', judul: 'Alpha', nilai: { 'c-number': 2 } })])
    const u = userEvent.setup()

    // Judul: Enter membuka editor berisi judul lama.
    await u.click(await cellOf('Alpha', COL.title))
    await u.keyboard('{Enter}')
    const title = await screen.findByRole('textbox', { name: 'Judul item Alpha' })
    await u.clear(title)
    await u.type(title, 'Alpha baru{Enter}')
    await waitFor(() => expect(writes().at(-1)).toMatchObject({ method: 'PATCH', path: '/items/a', body: { judul: 'Alpha baru' } }))
    expect(await screen.findByText('Alpha baru')).toBeInTheDocument()

    // Teks: mengetik langsung membuka editor dan menggantikan isi.
    await u.click(await cellOf('Alpha baru', COL.text))
    await u.keyboard('h')
    await u.keyboard('alo{Enter}')
    await waitFor(() => expect(writes().at(-1)).toMatchObject({ path: '/items/a/values/c-text', body: { nilai: 'halo' } }))

    // Angka: koma desimal diterima.
    await u.click(await cellOf('Alpha baru', COL.number))
    await u.keyboard('{Enter}')
    const num = await screen.findByRole('textbox', { name: 'Estimasi item Alpha baru' })
    expect(num).toHaveValue('2')
    await u.clear(num)
    await u.type(num, '3,5{Enter}')
    await waitFor(() => expect(writes().at(-1)).toMatchObject({ path: '/items/a/values/c-number', body: { nilai: 3.5 } }))

    // Progres.
    await u.click(await cellOf('Alpha baru', COL.progress))
    await u.keyboard('8')
    await u.keyboard('0%{Enter}')
    await waitFor(() => expect(writes().at(-1)).toMatchObject({ path: '/items/a/values/c-progress', body: { nilai: 80 } }))

    // Tanggal: pemilih tanggal bawaan.
    await u.click(await cellOf('Alpha baru', COL.date))
    await u.keyboard('{Enter}')
    const date = await screen.findByLabelText('Tenggat item Alpha baru')
    fireEvent.change(date, { target: { value: '2026-12-01' } })
    fireEvent.keyDown(date, { key: 'Enter' })
    await waitFor(() => expect(writes().at(-1)).toMatchObject({ path: '/items/a/values/c-date', body: { nilai: '2026-12-01' } }))
  })

  it('angka tidak sah ditolak di klien dan Esc membatalkan tanpa permintaan', async () => {
    const { writes } = setup([makeItem({ id: 'a', judul: 'Alpha' })])
    const u = userEvent.setup()
    await u.click(await cellOf('Alpha', COL.number))
    await u.keyboard('x')
    const input = await screen.findByRole('textbox', { name: 'Estimasi item Alpha' })
    await u.keyboard('{Enter}')
    expect(await screen.findByRole('alert')).toHaveTextContent('Masukkan angka')
    expect(input).toHaveAttribute('aria-invalid', 'true')
    await u.keyboard('{Escape}')
    expect(screen.queryByRole('textbox', { name: 'Estimasi item Alpha' })).not.toBeInTheDocument()

    await u.click(await cellOf('Alpha', COL.text))
    await u.keyboard('abc{Escape}')
    expect(writes()).toEqual([])
  })

  it('status, prioritas, person, dan timeline lewat panel pilihan', async () => {
    const { writes } = setup([makeItem({ id: 'a', judul: 'Alpha', nilai: { 'c-person': ['u-1'] } })])
    const u = userEvent.setup()

    await u.click(await cellOf('Alpha', COL.status))
    const statusList = await screen.findByRole('listbox', { name: 'Status item Alpha' })
    await u.click(within(statusList).getByText('Done'))
    await waitFor(() => expect(writes().at(-1)).toMatchObject({ path: '/items/a/values/c-status', body: { nilai: 'done' } }))
    expect(within(await cellOf('Alpha', COL.status)).getByText('Done')).toHaveClass('tag-hijau')

    // Prioritas dengan keyboard: Enter membuka, panah memilih, Enter menyimpan.
    await u.click(await cellOf('Alpha', COL.priority))
    await screen.findByRole('listbox', { name: 'Prioritas item Alpha' })
    await u.keyboard('{ArrowDown}{ArrowDown}{Enter}')
    await waitFor(() => expect(writes().at(-1)).toMatchObject({ path: '/items/a/values/c-priority', body: { nilai: 'tinggi' } }))

    await u.click(await cellOf('Alpha', COL.person))
    const dialog = await screen.findByRole('dialog', { name: 'Penanggung jawab item Alpha' })
    await u.click(within(dialog).getByRole('checkbox', { name: 'Sari Dewi' }))
    await u.click(within(dialog).getByRole('button', { name: 'Simpan' }))
    await waitFor(() =>
      expect(writes().at(-1)).toMatchObject({ path: '/items/a/values/c-person', body: { nilai: ['u-1', 'u-2'] } }),
    )

    await u.click(await cellOf('Alpha', COL.timeline))
    const tl = await screen.findByRole('dialog', { name: 'Timeline item Alpha' })
    fireEvent.change(within(tl).getByLabelText('Mulai'), { target: { value: '2026-11-01' } })
    fireEvent.change(within(tl).getByLabelText('Selesai'), { target: { value: '2026-10-01' } })
    await u.click(within(tl).getByRole('button', { name: 'Simpan' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Selesai tidak boleh sebelum mulai')
    fireEvent.change(within(tl).getByLabelText('Selesai'), { target: { value: '2026-11-20' } })
    await u.click(within(tl).getByRole('button', { name: 'Simpan' }))
    await waitFor(() =>
      expect(writes().at(-1)).toMatchObject({
        path: '/items/a/values/c-timeline',
        body: { nilai: { mulai: '2026-11-01', selesai: '2026-11-20' } },
      }),
    )
  })

  it('Delete mengosongkan sel; nilai yang ditolak server dikembalikan dengan pesan', async () => {
    const { writes } = setup([makeItem({ id: 'a', judul: 'Alpha', nilai: { 'c-text': 'lama', 'c-status': 'done' } })], {
      reject: (c) =>
        c.path.endsWith('/c-status') ? { status: 422, body: { error: 'validation', message: 'Label status tidak ada.' } } : null,
    })
    const u = userEvent.setup()
    await u.click(await cellOf('Alpha', COL.text))
    await u.keyboard('{Delete}')
    await waitFor(() => expect(writes().at(-1)).toMatchObject({ path: '/items/a/values/c-text', body: { nilai: null } }))
    expect(await cellOf('Alpha', COL.text)).toHaveTextContent('')

    await u.click(await cellOf('Alpha', COL.status))
    await u.click(within(await screen.findByRole('listbox')).getByText('Ready to start'))
    expect(await screen.findByRole('alert')).toHaveTextContent('Label status tidak ada.')
    await waitFor(async () => expect(within(await cellOf('Alpha', COL.status)).getByText('Done')).toBeInTheDocument())
  })

  it('Popover sel: Esc menutup dan fokus kembali ke sel asal, lalu panah tetap berpindah sel', async () => {
    const { writes } = setup([makeItem({ id: 'a', judul: 'Alpha' })])
    const u = userEvent.setup()

    // Daftar pilihan (status): dibuka dengan Enter dari keyboard.
    await u.click(await cellOf('Alpha', COL.title))
    await u.keyboard('{ArrowRight}{Enter}')
    expect(await screen.findByRole('listbox', { name: 'Status item Alpha' })).toBeInTheDocument()
    await u.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('listbox')).not.toBeInTheDocument())
    await waitFor(async () => expect(document.activeElement).toBe(await cellOf('Alpha', COL.status)))
    await u.keyboard('{ArrowRight}')
    expect(document.activeElement).toBe(await cellOf('Alpha', COL.person))

    // Panel person: Esc menutup dari dalam panel, fokus kembali ke sel person.
    await u.keyboard('{Enter}')
    const dialog = await screen.findByRole('dialog', { name: 'Penanggung jawab item Alpha' })
    await waitFor(() => expect(dialog).toContainElement(document.activeElement as HTMLElement))
    await u.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    await waitFor(async () => expect(document.activeElement).toBe(await cellOf('Alpha', COL.person)))
    await u.keyboard('{ArrowLeft}')
    expect(document.activeElement).toBe(await cellOf('Alpha', COL.status))
    expect(writes()).toEqual([])
  })

  it('Popover sel: Tab tidak lepas dari tabel', async () => {
    setup([makeItem({ id: 'a', judul: 'Alpha' })])
    const u = userEvent.setup()
    const grid = await screen.findByRole('grid')

    // Panel person: Tab berputar di antara isian panel (2 anggota, Batal, Simpan).
    await u.click(await cellOf('Alpha', COL.person))
    const dialog = await screen.findByRole('dialog', { name: 'Penanggung jawab item Alpha' })
    const first = within(dialog).getByRole('checkbox', { name: 'Budi' })
    await waitFor(() => expect(document.activeElement).toBe(first))
    await u.tab()
    await u.tab()
    await u.tab()
    expect(document.activeElement).toBe(within(dialog).getByRole('button', { name: 'Simpan' }))
    await u.tab()
    expect(document.activeElement).toBe(first)
    await u.tab({ shift: true })
    expect(document.activeElement).toBe(within(dialog).getByRole('button', { name: 'Simpan' }))
    await u.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())

    // Daftar pilihan: Tab menutup daftar dan fokus kembali ke sel, tetap di dalam tabel.
    await u.click(await cellOf('Alpha', COL.priority))
    await screen.findByRole('listbox', { name: 'Prioritas item Alpha' })
    await u.tab()
    await waitFor(() => expect(screen.queryByRole('listbox')).not.toBeInTheDocument())
    await waitFor(async () => expect(document.activeElement).toBe(await cellOf('Alpha', COL.priority)))
    expect(grid).toContainElement(document.activeElement as HTMLElement)
  })

  it('navigasi keyboard: panah, Home/End, dan melewati header grup', async () => {
    setup([makeItem({ id: 'a', judul: 'Alpha', urutan: 1 }), makeItem({ id: 'b', judul: 'Beta', urutan: 2 })])
    const u = userEvent.setup()
    await u.click(await cellOf('Alpha', COL.title))
    expect(document.activeElement).toBe(await cellOf('Alpha', COL.title))

    await u.keyboard('{ArrowRight}{ArrowRight}')
    expect(document.activeElement).toBe(await cellOf('Alpha', COL.person))
    await u.keyboard('{ArrowDown}')
    expect(document.activeElement).toBe(await cellOf('Beta', COL.person))
    await u.keyboard('{End}')
    expect(document.activeElement).toBe(await cellOf('Beta', COL.actions))
    await u.keyboard('{Home}{ArrowUp}{ArrowUp}')
    expect(document.activeElement).toHaveAttribute('aria-label', 'Grup Tugas, 2 item')
    expect(document.activeElement).toHaveAttribute('tabindex', '0')

    // Enter pada header grup melipat grup.
    await u.keyboard('{Enter}')
    expect(document.activeElement).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText('Alpha')).not.toBeInTheDocument()
  })

  it('Viewer: sel hanya-baca, tidak ada editor, menu aksi, atau baris tambah', async () => {
    const { writes } = setup([makeItem({ id: 'a', judul: 'Alpha', bisa_ubah: false, bisa_hapus: false })], {
      board: makeBoard({ peran_saya: 'viewer' }),
    })
    const u = userEvent.setup()
    const cell = await cellOf('Alpha', COL.text)
    expect(cell).toHaveAttribute('aria-readonly', 'true')
    await u.click(cell)
    await u.keyboard('{Enter}x{Delete}')
    await u.click(await cellOf('Alpha', COL.status))
    await u.click(await cellOf('Alpha', COL.actions))
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Item baru di Tugas')).not.toBeInTheDocument()
    expect(writes()).toEqual([])
  })

  it('kolom dependency: menampilkan predecessor, Enter membuka panel, Delete tidak menulis', async () => {
    const dep: Dependency = { id: 'ab', board_id: 'b1', predecessor_id: 'a', successor_id: 'b', tipe: 'FS', lag_hari: 0 }
    const { writes } = setup(
      [makeItem({ id: 'a', judul: 'Alpha' }), makeItem({ id: 'b', judul: 'Beta', urutan: 2048 })],
      { deps: [dep] },
    )
    const u = userEvent.setup()
    const cell = await cellOf('Beta', COL.dependency)
    await waitFor(() => expect(cell).toHaveTextContent('Alpha'))
    // Alpha tidak punya predecessor (baris dicari lewat Beta karena judul Alpha kini muncul dua kali).
    const rows = within(screen.getByRole('grid')).getAllByRole('row')
    const alphaRow = rows.find((r) => within(r).queryAllByRole('gridcell')[0]?.textContent === 'Alpha')
    expect(alphaRow && within(alphaRow).getAllByRole('gridcell')[COL.dependency]).toHaveTextContent('—')

    await u.click(cell)
    await u.keyboard('{Delete}')
    expect(writes()).toEqual([])
    await u.keyboard('{Enter}')
    const panel = await screen.findByRole('dialog', { name: 'Dependensi Beta' })
    expect(within(panel).getByRole('region', { name: 'Bergantung pada (predecessor)' })).toHaveTextContent('Alpha')
    await u.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    await waitFor(() => expect(document.activeElement).toBe(cell))
  })

  it('menu aksi memindah grup dan menghapus; baris tambah membuat item', async () => {
    const { writes } = setup([makeItem({ id: 'a', judul: 'Alpha' }), makeItem({ id: 'b', judul: 'Beta', urutan: 2048 })])
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const u = userEvent.setup()

    await u.click(await cellOf('Alpha', COL.actions))
    await u.click(within(await screen.findByRole('listbox', { name: 'Aksi item Alpha' })).getByText('Pindah ke Selesai'))
    await waitFor(() => expect(writes().at(-1)).toMatchObject({ path: '/items/a/move', body: { group_id: 'g2' } }))

    await u.click(await cellOf('Beta', COL.actions))
    await u.click(within(await screen.findByRole('listbox', { name: 'Aksi item Beta' })).getByText('Hapus item'))
    await waitFor(() => expect(writes().at(-1)).toMatchObject({ method: 'DELETE', path: '/items/b' }))
    await waitFor(() => expect(screen.queryByText('Beta')).not.toBeInTheDocument())

    await u.type(screen.getByLabelText('Item baru di Tugas'), 'Gamma{Enter}')
    await waitFor(() =>
      expect(writes().at(-1)).toMatchObject({ method: 'POST', path: '/boards/b1/items', body: { group_id: 'g1', judul: 'Gamma' } }),
    )
    expect(await screen.findByText('Gamma')).toBeInTheDocument()
  })

  it('board yang tidak dapat diakses menampilkan halaman tidak ditemukan', async () => {
    mockApi({ 'GET /auth/me': meReply(user()) })
    renderApp('/board/rahasia')
    expect(await screen.findByRole('heading', { name: 'Halaman tidak ditemukan' })).toBeInTheDocument()
  })
})
