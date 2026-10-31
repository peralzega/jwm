import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { AutomationRule, AutomationRun, Item } from '../api/types'
import { COL, makeBoard, makeItem, makeMember } from '../board/testData'
import { meReply, mockApi, renderApp, user, type Call } from '../test/utils'

const originals = {
  clientHeight: Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'clientHeight'),
  clientWidth: Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'clientWidth'),
}
beforeEach(() => {
  Object.defineProperty(HTMLElement.prototype, 'clientHeight', {
    configurable: true,
    get(this: HTMLElement) {
      return this.dataset.testid === 'board-scroll' ? 640 : 0
    },
  })
  Object.defineProperty(HTMLElement.prototype, 'clientWidth', {
    configurable: true,
    get(this: HTMLElement) {
      return this.dataset.testid === 'board-scroll' ? 1200 : 0
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

const rule = (over: Partial<AutomationRule> = {}): AutomationRule => ({
  id: 'r1',
  board_id: 'b1',
  nama: 'Done ke Selesai',
  pemicu: { jenis: 'status_berubah', ke: 'done' },
  kondisi: [{ jenis: 'kolom_sama', column_id: 'c-priority', nilai: 'tinggi' }],
  aksi: [{ jenis: 'pindah_grup', group_id: 'g2' }],
  aktif: true,
  dibuat_oleh_nama: 'Budi',
  dibuat_pada: '',
  diubah_pada: '',
  ...over,
})

function setup(opts: { manager?: boolean; rules?: AutomationRule[]; reject?: (c: Call) => { status: number; body: unknown } | null } = {}) {
  let rules = opts.rules ?? [rule()]
  const items: Item[] = [makeItem({ id: 'a', judul: 'Alpha' })]
  const runs: AutomationRun[] = [
    { id: 2, item_id: 'a', item_judul: 'Alpha', hasil: 'gagal', pesan: 'aksi 1: grup tujuan sudah dihapus', waktu: '2026-11-05T02:00:00Z' },
    { id: 1, item_id: null, item_judul: null, hasil: 'berhasil', pesan: 'dipindah ke grup "Selesai"', waktu: '2026-11-04T02:00:00Z' },
  ]
  const calls = mockApi({
    'GET /auth/me': meReply(user()),
    'GET /boards/b1': { status: 200, body: makeBoard({ peran_saya: opts.manager ? 'manager' : 'member' }) },
    'GET /boards/b1/items': { status: 200, body: { items, kursor_berikut: null } },
    'GET /projects/p1/members': { status: 200, body: { items: [makeMember({ user_id: 'u-1', nama: 'Budi' })] } },
    'GET /boards/b1/dependencies': { status: 200, body: { items: [] } },
    'GET /boards/b1/automations': () => ({ status: 200, body: { items: rules, bisa_kelola: !!opts.manager } }),
    'POST /boards/b1/automations': (c) => {
      const rejected = opts.reject?.(c)
      if (rejected) return rejected
      const r = rule({ ...(c.body as Partial<AutomationRule>), id: 'r2' })
      rules = [...rules, r]
      const webhook = r.aksi.some((a) => a.jenis === 'panggil_webhook')
      return { status: 201, body: webhook ? { ...r, webhook_rahasia: 'whsec_rahasia_uji' } : r }
    },
    'PATCH /automations/r1': (c) => {
      rules = rules.map((r) => (r.id === 'r1' ? { ...r, ...(c.body as Partial<AutomationRule>) } : r))
      return { status: 200, body: rules[0] }
    },
    'DELETE /automations/r1': () => {
      rules = rules.filter((r) => r.id !== 'r1')
      return { status: 204 }
    },
    'GET /automations/r1/runs': { status: 200, body: { items: runs } },
    'GET /automations/r1/webhooks': {
      status: 200,
      body: {
        items: [
          { id: 9, url: 'https://n8n.jiep.co.id/h', status: 'gagal', percobaan: 4, galat: 'penerima menjawab HTTP 500', kode_http: 500, dibuat_pada: '2026-11-05T02:00:00Z', selesai_pada: null },
        ],
      },
    },
    'POST /automations/r1/webhook-secret': { status: 200, body: { webhook_rahasia: 'whsec_baru' } },
    'PUT /items/a/values/c-status': (c) => ({ status: 200, body: { ...items[0], nilai: { 'c-status': (c.body as { nilai: string }).nilai } } }),
    'GET /items/a': { status: 200, body: { ...items[0], group_id: 'g2', nilai: { 'c-status': 'done' } } },
  })
  renderApp('/board/b1?tampilan=otomasi')
  return calls
}

describe('tab Otomasi', () => {
  it('anggota melihat aturan dalam kalimat dan riwayatnya, tanpa kontrol ubah', async () => {
    setup()
    const u = userEvent.setup()
    const list = await screen.findByRole('list', { name: 'Aturan otomasi' })
    expect(list).toHaveTextContent(
      'Saat status berubah menjadi Done, jika Prioritas = Tinggi, maka pindahkan ke grup Selesai.',
    )
    expect(screen.queryByRole('button', { name: 'Tambah aturan' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /^Ubah / })).not.toBeInTheDocument()
    expect(screen.getByText('Hanya manager proyek yang dapat mengubah aturan.')).toBeInTheDocument()

    await u.click(within(list).getByRole('button', { name: 'Riwayat Done ke Selesai' }))
    const table = await screen.findByRole('table', { name: 'Riwayat eksekusi Done ke Selesai' })
    expect(within(table).getByText('Gagal')).toHaveClass('text-danger')
    expect(table).toHaveTextContent('aksi 1: grup tujuan sudah dihapus')
    expect(table).toHaveTextContent('item terhapus')
  })

  it('manager membuat aturan lewat formulir', async () => {
    const calls = setup({ manager: true, rules: [] })
    const u = userEvent.setup()
    await u.click(await screen.findByRole('button', { name: 'Tambah aturan' }))
    const form = screen.getByRole('form', { name: 'Aturan otomasi' })
    await u.type(within(form).getByLabelText('Nama aturan'), 'Selesai otomatis')
    await u.selectOptions(within(form).getByLabelText('Menjadi status'), 'done')
    await u.click(within(form).getByRole('button', { name: 'Tambah kondisi' }))
    await u.selectOptions(within(form).getByLabelText('Kolom kondisi 1'), 'c-priority')
    await u.selectOptions(within(form).getByLabelText('Nilai kondisi 1'), 'tinggi')
    await u.selectOptions(within(form).getByLabelText('Grup aksi 1'), 'g2')
    await u.click(within(form).getByRole('button', { name: 'Tambah aksi' }))
    await u.selectOptions(within(form).getByLabelText('Kolom aksi 2'), 'c-person')
    await u.selectOptions(within(form).getByLabelText('Nilai aksi 2'), 'u-1')
    await u.click(within(form).getByRole('button', { name: 'Simpan aturan' }))
    await waitFor(() =>
      expect(calls.find((c) => c.method === 'POST')?.body).toEqual({
        nama: 'Selesai otomatis',
        pemicu: { jenis: 'status_berubah', ke: 'done' },
        kondisi: [{ jenis: 'kolom_sama', column_id: 'c-priority', nilai: 'tinggi' }],
        aksi: [
          { jenis: 'pindah_grup', group_id: 'g2' },
          { jenis: 'ubah_kolom', column_id: 'c-person', nilai: ['u-1'] },
        ],
        aktif: true,
      }),
    )
    await waitFor(() => expect(screen.queryByRole('form', { name: 'Aturan otomasi' })).not.toBeInTheDocument())
    expect(await screen.findByText('Selesai otomatis')).toBeInTheDocument()
  })

  it('galat validasi server tampil di formulir', async () => {
    setup({ manager: true, reject: () => ({ status: 422, body: { error: 'validation', message: 'aturan otomasi tidak valid: pemicu: kolom wajib dipilih' } }) })
    const u = userEvent.setup()
    await u.click(await screen.findByRole('button', { name: 'Tambah aturan' }))
    const form = screen.getByRole('form', { name: 'Aturan otomasi' })
    await u.type(within(form).getByLabelText('Nama aturan'), 'Tenggat')
    await u.selectOptions(within(form).getByLabelText('Jenis pemicu'), 'tanggal_tiba')
    await u.click(within(form).getByRole('button', { name: 'Simpan aturan' }))
    expect(await within(form).findByRole('alert')).toHaveTextContent('kolom wajib dipilih')
  })

  it('manager menonaktifkan dan menghapus aturan', async () => {
    const calls = setup({ manager: true })
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const u = userEvent.setup()
    await u.click(await screen.findByRole('button', { name: 'Nonaktifkan Done ke Selesai' }))
    await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')?.body).toMatchObject({ aktif: false, nama: 'Done ke Selesai' }))
    expect(await screen.findByRole('button', { name: 'Aktifkan Done ke Selesai' }, { timeout: 3000 })).toBeInTheDocument()
    expect(screen.getByRole('list', { name: 'Aturan otomasi' })).toHaveTextContent('(nonaktif)')
    await u.click(screen.getByRole('button', { name: 'Hapus Done ke Selesai' }))
    expect(await screen.findByText('Belum ada aturan otomasi di board ini.')).toBeInTheDocument()
  })

  it('board dengan aturan aktif memuat ulang item setelah perubahan nilai', async () => {
    const calls = setup()
    const u = userEvent.setup()
    await u.click(await screen.findByRole('tab', { name: 'Tabel' }))
    const text = await screen.findByText('Alpha', { selector: '[role=gridcell] span' })
    const row = text.closest<HTMLElement>('[role=row]')
    if (!row) throw new Error('baris tidak ada')
    await u.click(within(row).getAllByRole('gridcell')[COL.status] as HTMLElement)
    await u.click(within(await screen.findByRole('listbox')).getByText('Done'))
    await waitFor(() => expect(calls.some((c) => c.method === 'GET' && c.path === '/items/a')).toBe(true), { timeout: 3000 })
  })
})

describe('aksi webhook', () => {
  it('membuat aturan webhook; rahasia tampil sekali beserta cara verifikasi', async () => {
    const calls = setup({ manager: true, rules: [] })
    const u = userEvent.setup()
    await u.click(await screen.findByRole('button', { name: 'Tambah aturan' }))
    const form = screen.getByRole('form', { name: 'Aturan otomasi' })
    await u.type(within(form).getByLabelText('Nama aturan'), 'Kabari n8n')
    await u.selectOptions(within(form).getByLabelText('Jenis aksi 1'), 'panggil_webhook')
    await u.type(within(form).getByLabelText('URL webhook aksi 1'), 'https://n8n.jiep.co.id/webhook/abc')
    await u.click(within(form).getByRole('button', { name: 'Simpan aturan' }))
    await waitFor(() =>
      expect((calls.find((c) => c.method === 'POST')?.body as { aksi: unknown }).aksi).toEqual([
        { jenis: 'panggil_webhook', url: 'https://n8n.jiep.co.id/webhook/abc' },
      ]),
    )
    expect(await screen.findByLabelText('Rahasia webhook')).toHaveTextContent('whsec_rahasia_uji')
    expect(screen.getByText(/X-JWM-Signature/)).toBeInTheDocument()
    expect(await screen.findByText(/maka panggil webhook n8n\.jiep\.co\.id\./)).toBeInTheDocument()
    await u.click(screen.getByRole('button', { name: 'Tutup' }))
    expect(screen.queryByLabelText('Rahasia webhook')).not.toBeInTheDocument()
  })

  it('ganti rahasia dan riwayat kiriman webhook', async () => {
    setup({ manager: true, rules: [rule({ aksi: [{ jenis: 'panggil_webhook', url: 'https://n8n.jiep.co.id/h' }] })] })
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const u = userEvent.setup()
    await u.click(await screen.findByRole('button', { name: 'Ganti rahasia webhook Done ke Selesai' }))
    expect(await screen.findByLabelText('Rahasia webhook')).toHaveTextContent('whsec_baru')
    await u.click(screen.getByRole('button', { name: 'Riwayat Done ke Selesai' }))
    const table = await screen.findByRole('table', { name: 'Kiriman webhook Done ke Selesai' })
    expect(within(table).getByText('Gagal')).toHaveClass('text-danger')
    expect(table).toHaveTextContent('HTTP 500 penerima menjawab HTTP 500')
  })
})
