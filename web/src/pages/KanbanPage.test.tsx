import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import type { BoardDetail, Item } from '../api/types'
import { makeBoard, makeItem, makeMember } from '../board/testData'
import { meReply, mockApi, renderApp, user, type Call } from '../test/utils'

// jsdom tidak punya tata letak. Seret keyboard tidak bergantung geometri, tetapi
// dnd-kit tetap mengukur kolom dan kartu, dan virtualisasi butuh tinggi area gulir.
// Seret mouse/sentuh diuji di browser.
const COL_W = 300
const originalRect = Element.prototype.getBoundingClientRect
const originalClientHeight = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'clientHeight')

function rect(left: number, top: number, width: number, height: number): DOMRect {
  return { x: left, y: top, left, top, width, height, right: left + width, bottom: top + height, toJSON: () => ({}) }
}

beforeEach(() => {
  Element.prototype.getBoundingClientRect = function (this: Element) {
    let col = this.closest<HTMLElement>('[data-kanban-col]')
    // Salinan kartu saat diseret (DragOverlay) ada di luar kolom; di browser ia
    // muncul tepat di atas kartu asal (yang memegang fokus).
    const isOverlay = !col && !this.querySelector('[data-kanban-col]') && (this.closest('.kanban-card') ?? this.querySelector('.kanban-card'))
    if (isOverlay) {
      col = document.activeElement?.closest<HTMLElement>('[data-kanban-col]') ?? null
      if (col) {
        const i = [...document.querySelectorAll('[data-kanban-col]')].indexOf(col)
        return rect(i * COL_W + 8, 40, COL_W - 36, 64)
      }
    }
    if (!col) return rect(0, 0, 0, 0)
    const i = [...document.querySelectorAll('[data-kanban-col]')].indexOf(col)
    if (this === col) return rect(i * COL_W, 0, COL_W - 20, 600)
    return rect(i * COL_W + 8, 40, COL_W - 36, 64)
  }
  Object.defineProperty(HTMLElement.prototype, 'clientHeight', {
    configurable: true,
    get(this: HTMLElement) {
      return this.dataset.testid?.startsWith('kanban-scroll-') ? 600 : 0
    },
  })
  document.documentElement.dataset.density = 'compact'
})

afterEach(() => {
  Element.prototype.getBoundingClientRect = originalRect
  if (originalClientHeight) Object.defineProperty(HTMLElement.prototype, 'clientHeight', originalClientHeight)
})

const LONG = 'Migrasikan seluruh basis data keuangan lama ke klaster PostgreSQL baru beserta uji pemulihan'

const baseItems = (): Item[] => [
  makeItem({ id: 'a', judul: 'Alpha', urutan: 1, nilai: { 'c-status': 'ready_to_start', 'c-person': ['u-2'], 'c-priority': 'tinggi' } }),
  makeItem({ id: 'b', judul: 'Beta', urutan: 2, nilai: { 'c-status': 'done' } }),
  makeItem({ id: 'c', judul: LONG, urutan: 3 }),
]

interface Deferred {
  resolve: () => void
}

function setup(
  items: Item[],
  opts: { board?: BoardDetail; reply?: (c: Call) => { status: number; body?: unknown }; hold?: Deferred[]; path?: string } = {},
) {
  const board = opts.board ?? makeBoard()
  const store = new Map(items.map((it) => [it.id, it]))
  const handlers: Parameters<typeof mockApi>[0] = {
    'GET /auth/me': meReply(user()),
    'GET /boards/b1': { status: 200, body: board },
    'GET /boards/b1/items': { status: 200, body: { items, kursor_berikut: null } },
    'GET /projects/p1/members': {
      status: 200,
      body: { items: [makeMember({ user_id: 'u-1', nama: 'Budi' }), makeMember({ user_id: 'u-2', nama: 'Sari Dewi' })] },
    },
  }
  for (const it of items) {
    for (const col of board.kolom.filter((c) => c.tipe === 'status')) {
      handlers[`PUT /items/${it.id}/values/${col.id}`] = async (c) => {
        if (opts.hold) await new Promise<void>((resolve) => opts.hold?.push({ resolve }))
        if (opts.reply) return opts.reply(c)
        const cur = store.get(it.id) ?? it
        const v = (c.body as { nilai: unknown }).nilai
        const nilai = { ...cur.nilai }
        if (v === null) Reflect.deleteProperty(nilai, col.id)
        else nilai[col.id] = v
        const next = { ...cur, nilai }
        store.set(it.id, next)
        return { status: 200, body: next }
      }
    }
  }
  const calls = mockApi(handlers)
  const router = renderApp(opts.path ?? '/board/b1?tampilan=kanban')
  return { calls, router, puts: () => calls.filter((c) => c.method === 'PUT') }
}

async function column(judul: string): Promise<HTMLElement> {
  return screen.findByRole('region', { name: new RegExp(`^${judul}`) }).catch(async () => {
    const heading = await screen.findByRole('heading', { name: new RegExp(`^${judul}`) })
    const section = heading.closest<HTMLElement>('section')
    if (!section) throw new Error(`kolom ${judul} tidak ditemukan`)
    return section
  })
}

async function card(judul: string): Promise<HTMLElement> {
  return screen.findByRole('button', { name: new RegExp(`^${judul.slice(0, 20)}`) })
}

/** Seret dengan keyboard: angkat, pindah kolom sebanyak `steps` (+kanan, -kiri), lepas. */
async function dragByKeyboard(u: ReturnType<typeof userEvent.setup>, el: HTMLElement, steps: number, drop = '[Space]') {
  act(() => el.focus())
  await u.keyboard('[Space]')
  for (let i = 0; i < Math.abs(steps); i++) await u.keyboard(steps > 0 ? '[ArrowRight]' : '[ArrowLeft]')
  await u.keyboard(drop)
}

describe('Kanban', () => {
  it('satu kolom per label status plus "Tanpa status", dengan lencana berwarna dan jumlah kartu', async () => {
    setup(baseItems())
    expect(await screen.findByRole('tab', { name: 'Kanban' })).toHaveAttribute('aria-selected', 'true')
    const headings = (await screen.findAllByRole('heading', { level: 2 })).map((h) => h.textContent)
    expect(headings).toEqual(['Tanpa status1', 'Ready to start1', 'In progress0', 'Done1'])
    expect(within(await column('Done')).getByText('Done')).toHaveClass('tag-hijau', 'tag-fill')
    expect(within(await column('Tanpa status')).getByRole('button', { name: new RegExp(`^${LONG.slice(0, 20)}`) })).toBeInTheDocument()
    expect(within(await column('In progress')).getByText('Tidak ada kartu')).toBeInTheDocument()

    // Isi kartu: avatar penanggung jawab dan lencana prioritas.
    const alpha = await card('Alpha')
    expect(within(alpha).getByText('SD')).toHaveClass('size-(--avatar-size)')
    expect(within(alpha).getByText('Tinggi')).toHaveClass('tag-oranye', 'tag-fill')
  })

  it('judul panjang dipotong dengan elipsis dan judul lengkap ada di tooltip', async () => {
    setup(baseItems())
    const title = within(await card(LONG)).getByTestId('card-title')
    expect(title).toHaveClass('truncate', 'min-w-0')
    expect(title).toHaveAttribute('title', LONG)
    expect(title).toHaveTextContent(LONG)
  })

  for (const density of ['compact', 'normal'] as const) {
    it(`[${density}] seret dengan keyboard: kartu pindah lebih dulu, lalu tersimpan lewat API yang sama dengan tabel`, async () => {
      document.documentElement.dataset.density = density
      const hold: Deferred[] = []
      const { puts } = setup(baseItems(), { hold })
      const u = userEvent.setup()
      await dragByKeyboard(u, await card('Alpha'), 1)

      await waitFor(() => expect(puts()).toHaveLength(1))
      expect(puts()[0]).toMatchObject({ path: '/items/a/values/c-status', body: { nilai: 'in_progress' } })
      // Server belum menjawab, kartu sudah di kolom tujuan.
      expect(within(await column('In progress')).getByRole('button', { name: /^Alpha/ })).toBeInTheDocument()
      expect(within(await column('Ready to start')).queryByRole('button', { name: /^Alpha/ })).not.toBeInTheDocument()
      // Pengumuman untuk pembaca layar.
      expect(screen.getByTestId('kanban-live')).toHaveTextContent('Kartu Alpha dipindah ke kolom In progress.')
      // Fokus ikut ke kartu di kolom tujuan.
      await waitFor(async () => expect(document.activeElement).toBe(await card('Alpha')))

      act(() => hold.forEach((h) => h.resolve()))
      await waitFor(() => expect(within(screen.getByRole('heading', { name: /^In progress/ })).getByLabelText('1 kartu')).toBeInTheDocument())
    })
  }

  it('dikembalikan ke kolom asal dengan pesan bila server menolak', async () => {
    const { puts } = setup(baseItems(), {
      reply: () => ({ status: 422, body: { error: 'validation', message: 'Label status tidak ada di kolom ini.' } }),
    })
    const u = userEvent.setup()
    await dragByKeyboard(u, await card('Beta'), -1)
    await waitFor(() => expect(puts()).toHaveLength(1))
    expect(puts()[0]?.body).toEqual({ nilai: 'in_progress' })
    expect(await screen.findByRole('alert')).toHaveTextContent('Label status tidak ada di kolom ini.')
    await waitFor(async () => expect(within(await column('Done')).getByRole('button', { name: /^Beta/ })).toBeInTheDocument())
  })

  it('kolom tujuan disorot selama kartu diangkat dan bisa melompati beberapa kolom', async () => {
    const { puts } = setup(baseItems())
    const u = userEvent.setup()
    const alpha = await card('Alpha')
    act(() => alpha.focus())
    await u.keyboard('[Space]')
    expect(alpha).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByTestId('kanban-live')).toHaveTextContent('Kartu Alpha diangkat dari kolom Ready to start.')
    await u.keyboard('[ArrowRight][ArrowRight][ArrowRight]')
    expect(await column('Done')).toHaveClass('ring-primary')
    expect(await column('In progress')).not.toHaveClass('ring-primary')
    expect(screen.getByTestId('kanban-live')).toHaveTextContent('berada di atas kolom Done')
    await u.keyboard('{Enter}')
    await waitFor(() => expect(puts()[0]).toMatchObject({ body: { nilai: 'done' } }))
  })

  it('melepas di "Tanpa status" mengosongkan status (null)', async () => {
    const { puts } = setup(baseItems())
    await dragByKeyboard(userEvent.setup(), await card('Alpha'), -1)
    await waitFor(() => expect(puts()[0]).toMatchObject({ path: '/items/a/values/c-status', body: { nilai: null } }))
    expect(within(await column('Tanpa status')).getByRole('button', { name: /^Alpha/ })).toBeInTheDocument()
  })

  it('Esc membatalkan dan melepas di kolom yang sama tidak mengirim apa pun', async () => {
    const { puts } = setup(baseItems())
    const u = userEvent.setup()
    await dragByKeyboard(u, await card('Alpha'), 2, '[Escape]')
    await dragByKeyboard(u, await card('Alpha'), 0)
    expect(puts()).toEqual([])
    expect(within(await column('Ready to start')).getByRole('button', { name: /^Alpha/ })).toBeInTheDocument()
  })

  it('Viewer: kartu tidak dapat diseret', async () => {
    const items = baseItems().map((it) => ({ ...it, bisa_ubah: false, bisa_hapus: false }))
    const { puts } = setup(items, { board: makeBoard({ peran_saya: 'viewer' }) })
    const alpha = await card('Alpha')
    expect(alpha).toHaveAttribute('aria-disabled', 'true')
    expect(alpha).toHaveAccessibleName('Alpha (hanya baca)')
    await dragByKeyboard(userEvent.setup(), alpha, 1)
    expect(puts()).toEqual([])
    expect(within(await column('Ready to start')).getByRole('button', { name: /^Alpha/ })).toBeInTheDocument()
  })

  it('pilihan kolom status dan tab tersimpan di URL', async () => {
    const base = makeBoard()
    const qa = {
      id: 'c-qa',
      board_id: 'b1',
      tipe: 'status' as const,
      judul: 'Status QA',
      urutan: 20,
      konfigurasi: { labels: [{ id: 'lulus', judul: 'Lulus', warna: 'hijau' }] },
    }
    const items = baseItems().map((it) => (it.id === 'b' ? { ...it, nilai: { ...it.nilai, 'c-qa': 'lulus' } } : it))
    const { router } = setup(items, { board: { ...base, kolom: [...base.kolom, qa] } })
    const u = userEvent.setup()
    const select = await screen.findByLabelText('Kelompokkan menurut')
    expect(select).toHaveValue('c-status')
    await u.selectOptions(select, 'c-qa')
    expect(router.state.location.search).toContain('status=c-qa')
    expect((await screen.findAllByRole('heading', { level: 2 })).map((h) => h.textContent)).toEqual(['Tanpa status2', 'Lulus1'])

    await u.click(screen.getByRole('tab', { name: 'Tabel' }))
    expect(router.state.location.search).not.toContain('tampilan=')
    expect(await screen.findByRole('grid')).toBeInTheDocument()
    await u.keyboard('{ArrowRight}')
    // Radix Tabs memindah fokus (dan mengaktifkan tab) secara asinkron.
    await waitFor(() => expect(screen.getByRole('tab', { name: 'Kanban' })).toHaveAttribute('aria-selected', 'true'))
    expect(router.state.location.search).toContain('tampilan=kanban')
  })

  it('virtualisasi: 5.000 kartu dalam satu kolom hanya merender yang terlihat', async () => {
    const many = Array.from({ length: 5000 }, (_, i) => makeItem({ id: `i${i}`, judul: `Kartu ${i}`, urutan: i, nilai: { 'c-status': 'done' } }))
    setup(many)
    const done = await column('Done')
    expect(within(done).getByLabelText('5000 kartu')).toBeInTheDocument()
    const rendered = within(done).getAllByRole('button').length
    expect(rendered).toBeGreaterThan(3)
    expect(rendered).toBeLessThan(30)
    const scroller = screen.getByTestId('kanban-scroll-done')
    act(() => {
      scroller.scrollTop = 4000 * 68
      scroller.dispatchEvent(new Event('scroll'))
    })
    expect((await within(done).findAllByRole('button', { name: /^Kartu 40\d\d$/ })).length).toBeGreaterThan(0)
    expect(within(done).queryByRole('button', { name: /^Kartu 0$/ })).not.toBeInTheDocument()
  })
})
