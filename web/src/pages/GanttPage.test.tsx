import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import type { BoardDetail, Dependency, Item, ItemSchedule, ScheduleView } from '../api/types'
import { describeSpan, isoOf, todayNum } from '../board/gantt/scale'
import { makeBoard, makeItem } from '../board/testData'
import { meReply, mockApi, renderApp, user, type Call } from '../test/utils'

// jsdom tidak punya tata letak: area gulir Gantt diberi tinggi agar virtualisasi
// memasang baris. Token CSS tidak terbaca, jadi lebar hari skala minggu = 12 px.
const DAY_W = 12
const originals = {
  clientHeight: Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'clientHeight'),
  clientWidth: Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'clientWidth'),
}

beforeEach(() => {
  Object.defineProperty(HTMLElement.prototype, 'clientHeight', {
    configurable: true,
    get(this: HTMLElement) {
      return this.dataset.testid === 'gantt-scroll' ? 640 : 0
    },
  })
  Object.defineProperty(HTMLElement.prototype, 'clientWidth', {
    configurable: true,
    get(this: HTMLElement) {
      return this.dataset.testid === 'gantt-scroll' ? 1200 : 0
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

const T = todayNum()
const iso = (offset: number) => isoOf(T + offset)
const tl = (a: number, b: number) => ({ 'c-timeline': { mulai: iso(a), selesai: iso(b) } })
const span = (a: number, b: number) => describeSpan({ start: T + a, end: T + b })
const XSS = '<img src=x onerror=alert(1)>'

const baseItems = (): Item[] => [
  makeItem({ id: 'a', judul: 'Alpha', urutan: 1, nilai: tl(0, 4) }),
  makeItem({ id: 'b', judul: 'Beta', urutan: 2, nilai: tl(2, 6) }),
  makeItem({ id: 'c', judul: 'Gamma', urutan: 3 }),
  makeItem({ id: 'd', group_id: 'g2', judul: XSS, urutan: 1, nilai: tl(1, 1), bisa_ubah: false }),
  makeItem({ id: 'e', group_id: 'g2', judul: 'Kritis', urutan: 2, nilai: tl(3, 5) }),
]

const dep = (id: string, p: string, s: string, lag_hari = 0): Dependency => ({
  id,
  board_id: 'b1',
  predecessor_id: p,
  successor_id: s,
  tipe: 'FS',
  lag_hari,
})

// Hasil CPM tiruan (hanya untuk tes): backend menghitungnya dari dependensi dan kalender.
const sched = (item_id: string, over: Partial<ItemSchedule> = {}): ItemSchedule => ({
  item_id,
  es: null,
  ef: null,
  ls: null,
  lf: null,
  slack: null,
  kritis: false,
  melanggar_fs: false,
  tanpa_jadwal: false,
  ...over,
})

const scheduleView = (over: Partial<ScheduleView> = {}): ScheduleView => ({
  kolom_id: 'c-timeline',
  kolom_utama: true,
  selesai_proyek: null,
  items: [sched('e', { kritis: true, slack: 0 })],
  dependensi_dilanggar: [],
  peringatan: [],
  ...over,
})

function setup(
  items: Item[] = baseItems(),
  opts: {
    board?: BoardDetail
    deps?: Dependency[]
    schedule?: ScheduleView
    /** Successor yang ikut digeser oleh PUT (respons geser_otomatis). */
    shift?: Item['geser_otomatis']
    /** Rute tambahan, mis. POST/DELETE dependensi. */
    extra?: Parameters<typeof mockApi>[0]
    reject?: (c: Call) => { status: number; body: unknown } | null
    /** Tahan jawaban PUT sampai fungsi yang didorong ke larik ini dipanggil. */
    hold?: (() => void)[]
    path?: string
  } = {},
) {
  const store = new Map(items.map((it) => [it.id, it]))
  const board = opts.board ?? makeBoard()
  const handlers: Parameters<typeof mockApi>[0] = {
    'GET /auth/me': meReply(user()),
    'GET /boards/b1': { status: 200, body: board },
    'GET /boards/b1/items': { status: 200, body: { items, kursor_berikut: null } },
    'GET /projects/p1/members': { status: 200, body: { items: [] } },
    'GET /boards/b1/dependencies': { status: 200, body: { items: opts.deps ?? [dep('ab', 'a', 'b')] } },
    'GET /boards/b1/schedule': { status: 200, body: opts.schedule ?? scheduleView() },
    ...opts.extra,
  }
  for (const it of items) {
    handlers[`PUT /items/${it.id}/values/c-timeline`] = async (c) => {
      if (opts.hold) await new Promise<void>((resolve) => opts.hold?.push(resolve))
      const rejected = opts.reject?.(c)
      if (rejected) return rejected
      const cur = store.get(it.id) ?? it
      const next = { ...cur, nilai: { ...cur.nilai, 'c-timeline': (c.body as { nilai: unknown }).nilai } }
      store.set(it.id, next)
      return { status: 200, body: opts.shift ? { ...next, geser_otomatis: opts.shift } : next }
    }
  }
  const calls = mockApi(handlers)
  const router = renderApp(opts.path ?? '/board/b1?tampilan=gantt')
  return { calls, router, puts: () => calls.filter((c) => c.method === 'PUT') }
}

const bar = (name: string | RegExp) => screen.findByRole('button', { name: typeof name === 'string' ? new RegExp(`^${name}:`) : name })

describe('Gantt', () => {
  it('menampilkan batang, item tanpa jadwal, grup, dan garis hari ini', async () => {
    setup()
    const grid = await screen.findByRole('grid', { name: 'Gantt Board utama, 5 item' })
    expect(await bar('Alpha')).toHaveAccessibleName(`Alpha: ${span(0, 4)}`)
    expect(within(grid).getByRole('button', { name: 'Gamma: belum dijadwalkan' })).toHaveTextContent('Belum dijadwalkan')
    expect(within(grid).getByRole('button', { name: 'Grup Tugas, 3 item' })).toHaveAttribute('aria-expanded', 'true')
    expect(within(grid).getByRole('img', { name: 'Hari ini' })).toBeInTheDocument()
    // Lebar batang = durasi inklusif × lebar hari.
    expect((await bar('Alpha')).style.width).toBe(`${5 * DAY_W}px`)
  })

  it('judul berisi HTML tampil sebagai teks, bukan elemen', async () => {
    setup()
    expect(await bar(/^<img src=x onerror=alert\(1\)>:/)).toBeInTheDocument()
    expect(screen.getAllByText(XSS).length).toBeGreaterThan(0)
    expect(document.querySelector('img')).toBeNull()
  })

  it('kritis, slack, dan melanggar FS dari /schedule disorot dan disebut di label', async () => {
    setup(baseItems(), {
      schedule: scheduleView({
        items: [
          sched('e', { kritis: true, slack: 0 }),
          sched('a', { slack: 3 }),
          sched('b', { kritis: true, slack: 0, melanggar_fs: true }),
        ],
      }),
    })
    const k = await bar('Kritis')
    // Hasil /schedule tiba setelah item; label diperbarui setelahnya.
    await waitFor(() => expect(k).toHaveAccessibleName(`Kritis: ${span(3, 5)}, jalur kritis`))
    expect(k.className).toContain('bg-destructive')
    expect(await bar('Alpha')).toHaveAccessibleName(`Alpha: ${span(0, 4)}, slack 3 hari kerja`)
    const beta = await bar('Beta')
    expect(beta).toHaveAccessibleName(`Beta: ${span(2, 6)}, jalur kritis, mulai sebelum predecessor selesai`)
    expect(beta).toHaveAttribute('data-violates')
    expect(screen.getByText(/Batang merah: jalur kritis/)).toBeInTheDocument()
    expect(screen.getByText(/Garis putus-putus merah/)).toBeInTheDocument()
  })

  it('panah dependensi; pelanggaran FS dari backend (memperhitungkan lag) ditandai', async () => {
    // a→e tidak tumpang tindih di layar, tetapi backend menghitung lag dan menandainya.
    setup(baseItems(), {
      deps: [dep('ab', 'a', 'b'), dep('ae', 'a', 'e', 3), dep('ac', 'a', 'c')],
      schedule: scheduleView({ dependensi_dilanggar: ['ae'] }),
    })
    await bar('Alpha')
    const svg = await screen.findByTestId('gantt-arrows')
    // Gamma belum dijadwalkan: panahnya tidak digambar.
    await waitFor(() => expect(svg.querySelectorAll(':scope > path')).toHaveLength(2))
    await waitFor(() => expect(svg.querySelectorAll(':scope > path[data-violated]')).toHaveLength(1))
    expect(screen.getByText(/Garis putus-putus merah/)).toBeInTheDocument()
  })

  it('saat batang diseret, pelanggaran dihitung di klien sampai CPM dihitung ulang', async () => {
    const items = [makeItem({ id: 'a', judul: 'Alpha', nilai: tl(0, 2) }), makeItem({ id: 'b', judul: 'Beta', urutan: 2, nilai: tl(3, 4) })]
    setup(items)
    const beta = await bar('Beta')
    const svg = await screen.findByTestId('gantt-arrows')
    await waitFor(() => expect(svg.querySelectorAll(':scope > path')).toHaveLength(1))
    expect(svg.querySelector(':scope > path[data-violated]')).toBeNull()
    fireEvent.pointerDown(beta, { clientX: 100, button: 0, pointerId: 1 })
    fireEvent.pointerMove(beta, { clientX: 100 - 2 * DAY_W, pointerId: 1 })
    expect(svg.querySelector(':scope > path[data-violated]')).not.toBeNull()
  })

  it('tanpa pelanggaran: tidak ada catatan pelanggaran', async () => {
    const items = [makeItem({ id: 'a', judul: 'Alpha', nilai: tl(0, 2) }), makeItem({ id: 'b', judul: 'Beta', urutan: 2, nilai: tl(3, 4) })]
    setup(items)
    await bar('Alpha')
    const svg = await screen.findByTestId('gantt-arrows')
    await waitFor(() => expect(svg.querySelectorAll(':scope > path')).toHaveLength(1))
    expect(svg.querySelector(':scope > path[data-violated]')).toBeNull()
    expect(screen.queryByText(/Garis putus-putus merah/)).not.toBeInTheDocument()
  })

  it('keyboard: panah menggeser, Shift+panah mengubah selesai; tekan beruntun jadi satu simpan', async () => {
    const { puts } = setup()
    const u = userEvent.setup()
    const alpha = await bar('Alpha')
    act(() => alpha.focus())
    await u.keyboard('[ArrowRight][ArrowRight]')
    expect(alpha).toHaveAccessibleName(`Alpha: ${span(2, 6)}`)
    await waitFor(() => expect(puts()).toHaveLength(1), { timeout: 2000 })
    expect(puts()[0]).toMatchObject({ path: '/items/a/values/c-timeline', body: { nilai: { mulai: iso(2), selesai: iso(6) } } })

    await u.keyboard('{Shift>}[ArrowLeft]{/Shift}')
    await waitFor(() => expect(puts()).toHaveLength(2), { timeout: 2000 })
    expect(puts()[1]?.body).toEqual({ nilai: { mulai: iso(2), selesai: iso(5) } })
    expect(await bar('Alpha')).toHaveAccessibleName(`Alpha: ${span(2, 5)}`)
  })

  it('menggeser lagi saat simpan sebelumnya belum dijawab: tampilan tidak melompat kembali', async () => {
    const hold: (() => void)[] = []
    const { puts } = setup(baseItems(), { hold })
    const u = userEvent.setup()
    const alpha = await bar('Alpha')
    act(() => alpha.focus())
    await u.keyboard('[ArrowRight]')
    await waitFor(() => expect(hold).toHaveLength(1), { timeout: 2000 })
    await u.keyboard('[ArrowRight]')
    act(() => hold.shift()?.())
    await waitFor(() => expect(puts()[0]?.body).toEqual({ nilai: { mulai: iso(1), selesai: iso(5) } }))
    // Jawaban simpan pertama tidak menghapus draf kedua.
    expect(alpha).toHaveAccessibleName(`Alpha: ${span(2, 6)}`)
    await waitFor(() => expect(hold).toHaveLength(1), { timeout: 2000 })
    act(() => hold.shift()?.())
    await waitFor(() => expect(puts()).toHaveLength(2))
    expect(puts()[1]?.body).toEqual({ nilai: { mulai: iso(2), selesai: iso(6) } })
    await waitFor(() => expect(alpha).toHaveAccessibleName(`Alpha: ${span(2, 6)}`))
  })

  it('keyboard: Esc membatalkan perubahan yang belum disimpan', async () => {
    const { puts } = setup()
    const u = userEvent.setup()
    const alpha = await bar('Alpha')
    act(() => alpha.focus())
    await u.keyboard('[ArrowLeft][Escape]')
    expect(alpha).toHaveAccessibleName(`Alpha: ${span(0, 4)}`)
    await new Promise((r) => setTimeout(r, 800))
    expect(puts()).toHaveLength(0)
  })

  it('keyboard: pindah baris menyimpan perubahan yang tertunda dan memindah fokus', async () => {
    const { puts } = setup()
    const u = userEvent.setup()
    const alpha = await bar('Alpha')
    act(() => alpha.focus())
    await u.keyboard('[ArrowRight][ArrowDown]')
    await waitFor(() => expect(document.activeElement).toHaveAccessibleName(`Beta: ${span(2, 6)}`))
    await waitFor(() => expect(puts()).toHaveLength(1))
    expect(puts()[0]?.body).toEqual({ nilai: { mulai: iso(1), selesai: iso(5) } })
  })

  it('item hanya-baca tidak dapat digeser', async () => {
    const { puts } = setup()
    const u = userEvent.setup()
    const ro = await bar(/^<img/)
    expect(ro).toHaveAccessibleName(`${XSS}: ${span(1, 1)}, hanya baca`)
    act(() => ro.focus())
    await u.keyboard('[ArrowRight][Enter]')
    fireEvent.pointerDown(ro, { clientX: 100, button: 0, pointerId: 1 })
    fireEvent.pointerMove(ro, { clientX: 160, pointerId: 1 })
    fireEvent.pointerUp(ro, { clientX: 160, pointerId: 1 })
    await new Promise((r) => setTimeout(r, 800))
    expect(puts()).toHaveLength(0)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('Enter membuka editor tanggal; Esc menutup dan fokus kembali ke batang', async () => {
    setup()
    const u = userEvent.setup()
    const alpha = await bar('Alpha')
    act(() => alpha.focus())
    await u.keyboard('[Enter]')
    const dialog = await screen.findByRole('dialog', { name: 'Timeline item Alpha' })
    expect(within(dialog).getByLabelText(/Mulai/)).toHaveValue(iso(0))
    await u.keyboard('[Escape]')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    await waitFor(() => expect(document.activeElement).toHaveAccessibleName(/^Alpha:/))
  })

  it('seret mouse: batang dan ujungnya, dibulatkan ke hari', async () => {
    const { puts } = setup()
    const alpha = await bar('Alpha')
    fireEvent.pointerDown(alpha, { clientX: 100, button: 0, pointerId: 1 })
    fireEvent.pointerMove(alpha, { clientX: 100 + 2 * DAY_W + 4, pointerId: 1 })
    expect(alpha).toHaveAccessibleName(`Alpha: ${span(2, 6)}`)
    fireEvent.pointerUp(alpha, { pointerId: 1 })
    await waitFor(() => expect(puts()).toHaveLength(1))
    expect(puts()[0]?.body).toEqual({ nilai: { mulai: iso(2), selesai: iso(6) } })

    const beta = await bar('Beta')
    const end = beta.querySelector<HTMLElement>('[data-mode="selesai"]')
    if (!end) throw new Error('pegangan selesai tidak ada')
    fireEvent.pointerDown(end, { clientX: 300, button: 0, pointerId: 2 })
    fireEvent.pointerMove(beta, { clientX: 300 - 3 * DAY_W, pointerId: 2 })
    fireEvent.pointerUp(beta, { pointerId: 2 })
    await waitFor(() => expect(puts()).toHaveLength(2))
    expect(puts()[1]).toMatchObject({ path: '/items/b/values/c-timeline', body: { nilai: { mulai: iso(2), selesai: iso(3) } } })

    // Klik tanpa geser tidak menyimpan apa pun.
    const kritis = await bar('Kritis')
    fireEvent.pointerDown(kritis, { clientX: 50, button: 0, pointerId: 3 })
    fireEvent.pointerUp(kritis, { pointerId: 3 })
    await new Promise((r) => setTimeout(r, 50))
    expect(puts()).toHaveLength(2)
  })

  it('server menolak: batang kembali dan galat tampil', async () => {
    const { puts } = setup(baseItems(), {
      reject: () => ({ status: 422, body: { error: 'validation', message: 'Jadwal ditolak server.' } }),
    })
    const alpha = await bar('Alpha')
    fireEvent.pointerDown(alpha, { clientX: 0, button: 0, pointerId: 1 })
    fireEvent.pointerMove(alpha, { clientX: 5 * DAY_W, pointerId: 1 })
    fireEvent.pointerUp(alpha, { pointerId: 1 })
    expect(await screen.findByRole('alert')).toHaveTextContent('Jadwal ditolak server.')
    await waitFor(() => expect(screen.getByRole('button', { name: /^Alpha:/ })).toHaveAccessibleName(`Alpha: ${span(0, 4)}`))
    expect(puts()).toHaveLength(1)
  })

  it('melipat grup menyembunyikan batangnya', async () => {
    setup()
    const u = userEvent.setup()
    await bar('Alpha')
    await u.click(screen.getByRole('button', { name: 'Grup Tugas, 3 item' }))
    expect(screen.queryByRole('button', { name: /^Alpha:/ })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Grup Tugas, 3 item' })).toHaveAttribute('aria-expanded', 'false')
  })

  it('skala waktu tersimpan di URL dan mengubah lebar hari', async () => {
    const { router } = setup()
    const u = userEvent.setup()
    await bar('Alpha')
    const group = screen.getByRole('group', { name: 'Skala waktu' })
    expect(within(group).getByRole('button', { name: 'Minggu' })).toHaveAttribute('aria-pressed', 'true')
    await u.click(within(group).getByRole('button', { name: 'Hari' }))
    expect(router.state.location.search).toContain('skala=hari')
    expect(within(group).getByRole('button', { name: 'Hari' })).toHaveAttribute('aria-pressed', 'true')
    await waitFor(async () => expect((await bar('Alpha')).style.width).toBe(`${5 * 28}px`))
  })

  it('board tanpa kolom timeline: pesan jelas', async () => {
    const board = makeBoard()
    setup([], { board: { ...board, kolom: board.kolom.filter((c) => c.tipe !== 'timeline') } })
    expect(await screen.findByText('Board ini belum memiliki kolom timeline untuk Gantt.')).toBeInTheDocument()
  })

  it('lebih dari satu kolom timeline: dapat dipilih', async () => {
    const board = makeBoard()
    const first = board.kolom.find((c) => c.tipe === 'timeline')
    if (!first) throw new Error('fixture tanpa kolom timeline')
    const extra = { ...first, id: 'c-tl2', judul: 'Rencana awal', urutan: 10 }
    const items = [makeItem({ id: 'a', judul: 'Alpha', nilai: { ...tl(0, 1), 'c-tl2': { mulai: iso(5), selesai: iso(9) } } })]
    const { router } = setup(items, { board: { ...board, kolom: [...board.kolom, extra] }, deps: [] })
    const u = userEvent.setup()
    expect(await bar('Alpha')).toHaveAccessibleName(`Alpha: ${span(0, 1)}`)
    expect(screen.queryByRole('note')).not.toBeInTheDocument()
    await u.selectOptions(screen.getByRole('combobox', { name: 'Kolom timeline' }), 'c-tl2')
    expect(router.state.location.search).toContain('timeline=c-tl2')
    await waitFor(async () => expect(await bar('Alpha')).toHaveAccessibleName(`Alpha: ${span(5, 9)}`))
    // Kolom bukan utama: pergeseran otomatis tidak berlaku.
    expect(screen.getByRole('note')).toHaveTextContent(
      `Pergeseran otomatis successor tidak berlaku pada kolom Rencana awal. Pergeseran hanya terjadi saat jadwal di kolom timeline utama (${first.judul}) berubah.`,
    )
  })

  it('peringatan kalender kerja yang belum diisi tampil', async () => {
    setup(baseItems(), {
      schedule: scheduleView({
        peringatan: [{ kode: 'kalender_kosong', tahun: 2027, pesan: 'Kalender kerja 2027 belum diisi. Hanya Sabtu dan Minggu yang dihitung libur.' }],
        selesai_proyek: '2027-01-05',
      }),
    })
    expect(await screen.findByText(/Kalender kerja 2027 belum diisi/)).toBeInTheDocument()
    expect(screen.getByText(/Perkiraan selesai: 5 Jan 2027/)).toBeInTheDocument()
  })

  it('pergeseran otomatis: successor diperbarui dan diumumkan; jadwal dihitung ulang', async () => {
    const { calls } = setup(baseItems(), {
      shift: [{ id: 'b', judul: 'Beta', mulai: iso(5), selesai: iso(9), predecessor_id: 'a' }],
    })
    const u = userEvent.setup()
    const alpha = await bar('Alpha')
    await waitFor(() => expect(calls.filter((c) => c.path === '/boards/b1/schedule')).toHaveLength(1))
    act(() => alpha.focus())
    await u.keyboard('{Shift>}[ArrowRight]{/Shift}')
    expect(await screen.findByText(/1 item ikut bergeser agar urutan dependensi terpenuhi: Beta/)).toBeInTheDocument()
    expect(await bar('Beta')).toHaveAccessibleName(`Beta: ${span(5, 9)}`)
    await waitFor(() => expect(calls.filter((c) => c.path === '/boards/b1/schedule')).toHaveLength(2))
    await u.click(screen.getByRole('button', { name: 'Tutup' }))
    expect(screen.queryByText(/ikut bergeser/)).not.toBeInTheDocument()
  })

  it('D membuka panel dependensi: tambah dan hapus predecessor, galat siklus tampil', async () => {
    let deps = [dep('ab', 'a', 'b')]
    const { calls } = setup(baseItems(), {
      extra: {
        'GET /boards/b1/dependencies': () => ({ status: 200, body: { items: deps } }),
        'POST /boards/b1/dependencies': (c) => {
          const body = c.body as { predecessor_id: string; successor_id: string; lag_hari: number }
          if (body.predecessor_id === 'e') {
            return { status: 422, body: { error: 'dependency_cycle', message: 'Dependensi ditolak karena membentuk siklus: "Beta" sudah mendahului "Kritis".' } }
          }
          const d = dep('nb', body.predecessor_id, body.successor_id, body.lag_hari)
          deps = [...deps, d]
          return { status: 201, body: d }
        },
        'DELETE /dependencies/ab': () => {
          deps = deps.filter((d) => d.id !== 'ab')
          return { status: 204 }
        },
      },
    })
    const u = userEvent.setup()
    const beta = await bar('Beta')
    act(() => beta.focus())
    await u.keyboard('d')
    const panel = await screen.findByRole('dialog', { name: 'Dependensi Beta' })
    const preds = within(panel).getByRole('region', { name: 'Bergantung pada (predecessor)' })
    expect(within(preds).getByText('Alpha')).toBeInTheDocument()

    // Tambah Gamma dengan lag 2 hari kerja.
    await u.clear(within(panel).getByLabelText('Lag (hari kerja)'))
    await u.type(within(panel).getByLabelText('Lag (hari kerja)'), '2')
    await u.type(within(panel).getByLabelText('Tambah predecessor'), 'gam')
    await u.click(within(panel).getByRole('button', { name: 'Tambahkan Gamma sebagai predecessor' }))
    await waitFor(() =>
      expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ predecessor_id: 'c', successor_id: 'b', lag_hari: 2 }),
    )
    expect(await within(preds).findByText('Gamma')).toBeInTheDocument()
    expect(within(preds).getByText(/lag 2 hari kerja/)).toBeInTheDocument()

    // Siklus ditolak server: pesan tampil di panel.
    await u.type(within(panel).getByLabelText('Tambah predecessor'), 'krit')
    await u.click(within(panel).getByRole('button', { name: 'Tambahkan Kritis sebagai predecessor' }))
    expect(await within(panel).findByRole('alert')).toHaveTextContent('membentuk siklus')

    await u.click(within(panel).getByRole('button', { name: 'Hapus dependensi dari Alpha' }))
    await waitFor(() => expect(within(preds).queryByText('Alpha')).not.toBeInTheDocument())
    // Daftar dependensi dan jadwal dimuat ulang setelah perubahan.
    expect(calls.filter((c) => c.path === '/boards/b1/schedule').length).toBeGreaterThan(1)

    await u.keyboard('[Escape]')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    await waitFor(() => expect(document.activeElement).toHaveAccessibleName(/^Beta:/))
  })

  it('item hanya-baca: panel dependensi tanpa kontrol ubah', async () => {
    setup()
    const u = userEvent.setup()
    await bar(/^<img/)
    await u.click(screen.getByRole('button', { name: `Dependensi ${XSS}` }))
    const panel = await screen.findByRole('dialog', { name: `Dependensi ${XSS}` })
    expect(within(panel).getByText('Anda tidak berhak mengubah dependensi item ini.')).toBeInTheDocument()
    expect(within(panel).queryByLabelText('Tambah predecessor')).not.toBeInTheDocument()
  })
})
