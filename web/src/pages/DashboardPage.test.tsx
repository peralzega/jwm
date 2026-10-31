import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { DashboardPanels } from '../api/types'
import { meReply, mockApi, renderApp, user } from '../test/utils'

// EventSource tiruan: jsdom tidak menyediakannya.
class FakeEventSource {
  static instances: FakeEventSource[] = []
  static CLOSED = 2
  readyState = 1
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  closed = false
  private listeners = new Map<string, (() => void)[]>()
  constructor(public url: string) {
    FakeEventSource.instances.push(this)
  }
  addEventListener(type: string, fn: () => void) {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), fn])
  }
  emit(type: string) {
    for (const fn of this.listeners.get(type) ?? []) fn()
  }
  close() {
    this.closed = true
  }
}

beforeEach(() => {
  FakeEventSource.instances = []
  vi.stubGlobal('EventSource', FakeEventSource)
})
afterEach(() => {
  vi.unstubAllGlobals()
})

const panels = (over: Partial<DashboardPanels> = {}): DashboardPanels => ({
  project_id: null,
  hari_ini: '2026-11-05',
  jumlah_item: 6,
  status: [
    { label: 'Done', warna: 'hijau', jumlah: 2 },
    { label: 'In progress', warna: 'oranye', jumlah: 3 },
  ],
  tanpa_status: 1,
  beban: [
    { user_id: 'u-1', nama: 'Budi', item_terbuka: 2, hari_kerja: 8, terlambat: 1 },
    { user_id: 'u-2', nama: '=Sari', item_terbuka: 1, hari_kerja: 5, terlambat: 0 },
  ],
  belum_ditugaskan: 2,
  tren: Array.from({ length: 12 }, (_, i) => ({
    minggu_mulai: new Date(Date.UTC(2026, 7, 17 + 7 * i)).toISOString().slice(0, 10),
    selesai: i === 11 ? 1 : i === 9 ? 2 : 0,
  })),
  keuangan: [
    { project_id: 'p1', nama: 'Jaringan', anggaran: '1500000.50', realisasi: '250000', sisa: '1250000.50', jumlah_entri: 3 },
    { project_id: 'p2', nama: 'Gedung', anggaran: '100', realisasi: '200', sisa: '-100', jumlah_entri: 2 },
  ],
  ...over,
})

function setup(path = '/dasbor') {
  const calls = mockApi({
    'GET /auth/me': meReply(user()),
    'GET /projects': {
      status: 200,
      body: { items: [{ id: 'p1', nama: 'Jaringan' }, { id: 'p2', nama: 'Gedung' }] },
    },
    'GET /dashboard': (c) => ({ status: 200, body: panels({ project_id: c.path.includes('p1') ? 'p1' : null }) }),
  })
  const router = renderApp(path)
  return { calls, router, dashCalls: () => calls.filter((c) => c.path === '/dashboard') }
}

describe('dasbor', () => {
  it('menampilkan empat panel dengan angka sebagai teks', async () => {
    setup()
    const status = await screen.findByRole('region', { name: 'Sebaran status' })
    expect(status).toHaveTextContent('Done2 (33%)')
    expect(status).toHaveTextContent('Tanpa status1 (17%)')
    const beban = screen.getByRole('region', { name: 'Beban kerja per orang' })
    expect(within(beban).getAllByRole('row')[1]).toHaveTextContent('Budi281')
    expect(beban).toHaveTextContent('2 item terbuka belum punya penanggung jawab.')
    const tren = screen.getByRole('region', { name: 'Tren penyelesaian' })
    expect(tren).toHaveTextContent('3 item selesai dalam 12 minggu terakhir')
    expect(within(tren).getAllByRole('listitem')).toHaveLength(12)
    expect(within(tren).getByLabelText(/2 Nov 2026: 1 item/)).toBeInTheDocument()
    const keu = screen.getByRole('region', { name: 'Keuangan' })
    expect(keu).toHaveTextContent(/Jaringan\s*Rp\s1\.500\.000,5/)
    expect(within(keu).getByText(/-Rp\s100/)).toHaveClass('text-danger')
  })

  it('lingkup proyek tersimpan di URL dan dipakai API', async () => {
    const { router, dashCalls } = setup()
    const u = userEvent.setup()
    await screen.findByRole('region', { name: 'Sebaran status' })
    await u.selectOptions(screen.getByLabelText('Lingkup'), 'p1')
    expect(router.state.location.search).toBe('?proyek=p1')
    await waitFor(() => expect(dashCalls().at(-1)?.path).toBe('/dashboard'))
    expect(FakeEventSource.instances.at(-1)?.url).toBe('/api/v1/stream?project=p1')
    expect(FakeEventSource.instances[0]?.closed).toBe(true)
  })

  it('event SSE "berubah" memuat ulang panel; status sambungan diumumkan', async () => {
    const { dashCalls } = setup()
    await screen.findByRole('region', { name: 'Sebaran status' })
    const es = FakeEventSource.instances[0]
    if (!es) throw new Error('EventSource tidak dibuat')
    expect(es.url).toBe('/api/v1/stream')
    expect(screen.getByText('Menyambung pembaruan langsung…')).toBeInTheDocument()
    act(() => es.onopen?.())
    expect(screen.getByText('Pembaruan langsung aktif')).toBeInTheDocument()
    const before = dashCalls().length
    act(() => {
      es.emit('berubah')
      es.emit('berubah')
    })
    await waitFor(() => expect(dashCalls().length).toBe(before + 1))
  })

  it('unduh CSV per panel dan cetak', async () => {
    setup()
    const u = userEvent.setup()
    const blobs: Blob[] = []
    vi.stubGlobal('URL', Object.assign(URL, {
      createObjectURL: (b: Blob) => {
        blobs.push(b)
        return 'blob:uji'
      },
      revokeObjectURL: () => undefined,
    }))
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => undefined)
    const print = vi.spyOn(window, 'print').mockImplementation(() => undefined)
    await screen.findByRole('region', { name: 'Beban kerja per orang' })
    await u.click(screen.getByRole('button', { name: 'Unduh CSV beban kerja' }))
    expect(click).toHaveBeenCalledTimes(1)
    const blob = blobs[0]
    if (!blob) throw new Error('CSV tidak dibuat')
    // BOM UTF-8 di awal berkas (Blob.text() membuangnya saat decode).
    expect([...new Uint8Array(await blob.arrayBuffer()).slice(0, 3)]).toEqual([0xef, 0xbb, 0xbf])
    expect(await blob.text()).toBe("Nama;Item terbuka;Hari kerja;Terlambat\r\nBudi;2;8;1\r\n'=Sari;1;5;0\r\n")
    await u.click(screen.getByRole('button', { name: 'Unduh CSV keuangan' }))
    expect(await blobs[1]?.text()).toContain('Gedung;100;200;-100;2')
    await u.click(screen.getByRole('button', { name: 'Cetak / simpan PDF' }))
    expect(print).toHaveBeenCalled()
  })
})
