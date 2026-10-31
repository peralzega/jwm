import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import type { Portfolio, PortfolioDashboard, ProjectHealth, RiskItem } from '../api/types'
import { meReply, mockApi, renderApp, user } from '../test/utils'

const rules = { lewat_tenggat: 0, tenggat_dekat: 0, tanpa_penanggung_jawab: 0, predecessor_terlambat: 0, melewati_tenggat_proyek: 0 }

function ph(over: Partial<ProjectHealth>): ProjectHealth {
  return {
    id: 'p1',
    nama: 'Proyek',
    pemilik_id: 'u-1',
    pemilik_nama: 'Budi',
    prioritas: 'sedang',
    mulai: null,
    selesai: null,
    portfolio_id: null,
    portfolio_nama: null,
    progres: 50,
    progres_rencana: 60,
    progres_jadwal: 50,
    jumlah_item: 10,
    item_berisiko: 0,
    per_aturan: rules,
    kritis_terlambat: 0,
    health: 'hijau',
    health_otomatis: 'hijau',
    health_alasan: ['0% item berisiko; progres sesuai rencana.'],
    status_manual: null,
    peringatan: [],
    ...over,
  }
}

const pf: Portfolio = { id: 'pf1', nama: 'Infrastruktur', jumlah_proyek: 1, dibuat_pada: '', diubah_pada: '' }

const items = (): ProjectHealth[] => [
  ph({
    id: 'a',
    nama: 'Jaringan',
    prioritas: 'tinggi',
    portfolio_id: 'pf1',
    portfolio_nama: 'Infrastruktur',
    progres: 39.4,
    progres_rencana: 56.3,
    item_berisiko: 2,
    jumlah_item: 5,
    per_aturan: { ...rules, lewat_tenggat: 1, tenggat_dekat: 1, tanpa_penanggung_jawab: 1 },
    health: 'merah',
    health_otomatis: 'merah',
    health_alasan: ['40% item berisiko (Merah bila > 25%).'],
  }),
  ph({ id: 'b', nama: 'Gedung', pemilik_id: 'u-2', pemilik_nama: 'Sari', prioritas: 'rendah', health: 'kuning', health_otomatis: 'kuning' }),
  ph({
    id: 'c',
    nama: 'Aplikasi',
    health: 'hijau',
    health_otomatis: 'merah',
    status_manual: { status: 'hijau', alasan: 'Vendor konfirmasi.', oleh_nama: 'PMO Satu', pada: '2026-11-05T02:00:00Z' },
  }),
]

function setup(peran: 'pmo' | 'member' = 'member', path = '/portofolio') {
  let data = items()
  const risks: RiskItem[] = [
    {
      item_id: 'i1',
      board_id: 'b1',
      board_nama: 'Board Jaringan',
      judul: 'Pengadaan',
      kritis: false,
      alasan: [
        { kode: 'lewat_tenggat', pesan: 'Lewat tenggat 2026-11-04 dan belum Done.' },
        { kode: 'tanpa_penanggung_jawab', pesan: 'Belum ada penanggung jawab.' },
      ],
    },
  ]
  const calls = mockApi({
    'GET /auth/me': meReply(user({ peran })),
    'GET /portfolio': (): { status: number; body: PortfolioDashboard } => ({
      status: 200,
      body: {
        items: data,
        ambang: { tenggat_dekat_hari: 3, tenggat_dekat_progres: 50, kuning_persen: 10, merah_persen: 25, tertinggal_poin: 10 },
        hari_ini: '2026-11-05',
      },
    }),
    'GET /portfolios': { status: 200, body: { items: [pf] } },
    'GET /portfolio/projects/a/risks': { status: 200, body: { project_id: 'a', items: risks } },
    'PUT /projects/b/status-manual': (c) => {
      const body = c.body as { status: 'merah'; alasan: string }
      data = data.map((p) =>
        p.id === 'b' ? { ...p, health: body.status, status_manual: { ...body, oleh_nama: 'Budi', pada: '' } } : p,
      )
      return { status: 204 }
    },
    'DELETE /projects/c/status-manual': () => {
      data = data.map((p) => (p.id === 'c' ? { ...p, health: p.health_otomatis, status_manual: null } : p))
      return { status: 204 }
    },
    'POST /portfolios': { status: 201, body: { ...pf, id: 'pf2', nama: 'Digital' } },
  })
  const router = renderApp(path)
  return { calls, router }
}

const table = () => screen.findByRole('table', { name: 'Proyek, progres, dan health' })
const row = async (nama: string) => {
  const link = await within(await table()).findByRole('link', { name: nama })
  const tr = link.closest('tr')
  if (!tr) throw new Error(`baris ${nama} tidak ada`)
  return tr
}

describe('dasbor portofolio', () => {
  it('menampilkan progres, prioritas, health, dan tanda manual', async () => {
    setup()
    const j = await row('Jaringan')
    expect(j).toHaveTextContent('Infrastruktur')
    expect(within(j).getByText('Tinggi')).toHaveClass('tag-oranye', 'tag-fill')
    expect(j).toHaveTextContent('39,4% (rencana 56,3%)')
    expect(within(j).getByText('Merah')).toHaveClass('tag-merah')
    const a = await row('Aplikasi')
    expect(within(a).getByText('Hijau')).toBeInTheDocument()
    expect(within(a).getByText('manual')).toHaveAttribute('title', 'Ditimpa PMO Satu: Vendor konfirmasi.')
    expect(screen.getByRole('status')).toHaveTextContent('3 proyek · 1 Merah · 1 Kuning · 1 Hijau')
    // Bukan Admin/PMO: tanpa tombol timpa dan panel kelola portofolio.
    expect(screen.queryByRole('button', { name: /^Timpa status/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('region', { name: 'Kelola portofolio' })).not.toBeInTheDocument()
  })

  it('rincian: alasan health, risiko per aturan, dan item berisiko beserta alasannya', async () => {
    setup()
    const u = userEvent.setup()
    await u.click(within(await row('Jaringan')).getByRole('button', { name: 'Rincian Jaringan: 2 dari 5 item berisiko' }))
    const health = screen.getByRole('region', { name: 'Health Jaringan' })
    expect(health).toHaveTextContent('40% item berisiko (Merah bila > 25%).')
    const perRule = screen.getByRole('region', { name: 'Risiko per aturan Jaringan' })
    expect(within(perRule).getByText('Lewat tenggat').nextSibling).toHaveTextContent('1')
    const list = await screen.findByRole('list', { name: 'Item berisiko' })
    expect(within(list).getByRole('link', { name: 'Pengadaan' })).toHaveAttribute('href', '/board/b1')
    expect(list).toHaveTextContent('Lewat tenggat 2026-11-04 dan belum Done.')
    expect(list).toHaveTextContent('Belum ada penanggung jawab.')
  })

  it('filter tersimpan di URL', async () => {
    const { router } = setup('member', '/portofolio?status=merah')
    await row('Jaringan')
    expect(within(await table()).queryByRole('link', { name: 'Gedung' })).not.toBeInTheDocument()
    const u = userEvent.setup()
    const filters = screen.getByRole('group', { name: 'Filter' })
    await u.selectOptions(within(filters).getByLabelText('Status'), '')
    await u.selectOptions(within(filters).getByLabelText('Pemilik'), 'u-2')
    expect(router.state.location.search).toBe('?pemilik=u-2')
    await waitFor(() => expect(within(screen.getByRole('table')).getAllByRole('link')).toHaveLength(1))
    expect(within(screen.getByRole('table')).getByRole('link', { name: 'Gedung' })).toBeInTheDocument()
    await u.selectOptions(within(filters).getByLabelText('Pemilik'), '')
    await u.selectOptions(within(filters).getByLabelText('Portofolio'), 'tanpa')
    await waitFor(() => expect(within(screen.getByRole('table')).getAllByRole('link')).toHaveLength(2))
    await u.selectOptions(within(filters).getByLabelText('Prioritas'), 'kritis')
    expect(await screen.findByText('Tidak ada proyek yang cocok dengan filter.')).toBeInTheDocument()
  })

  it('PMO menimpa status dengan alasan wajib, lalu mengembalikan ke otomatis', async () => {
    const { calls } = setup('pmo')
    const u = userEvent.setup()
    await u.click(within(await row('Gedung')).getByRole('button', { name: 'Timpa status Gedung' }))
    const form = screen.getByRole('form', { name: 'Timpa status Gedung' })
    const save = within(form).getByRole('button', { name: 'Simpan status' })
    expect(save).toBeDisabled()
    await u.selectOptions(within(form).getByLabelText('Status'), 'merah')
    await u.type(within(form).getByLabelText('Alasan (wajib)'), 'Anggaran dibekukan.')
    await u.click(save)
    await waitFor(() =>
      expect(calls.find((c) => c.method === 'PUT')?.body).toEqual({ status: 'merah', alasan: 'Anggaran dibekukan.' }),
    )
    await waitFor(async () => expect(within(await row('Gedung')).getByText('Merah')).toBeInTheDocument())
    expect(screen.queryByRole('form', { name: 'Timpa status Gedung' })).not.toBeInTheDocument()

    await u.click(within(await row('Aplikasi')).getByRole('button', { name: 'Timpa status Aplikasi' }))
    await u.click(within(screen.getByRole('form', { name: 'Timpa status Aplikasi' })).getByRole('button', { name: 'Kembali ke otomatis' }))
    await waitFor(async () => expect(within(await row('Aplikasi')).getByText('Merah')).toBeInTheDocument())
    expect(within(await row('Aplikasi')).queryByText('manual')).not.toBeInTheDocument()
  })

  it('PMO menambah portofolio', async () => {
    const { calls } = setup('pmo')
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const u = userEvent.setup()
    const panel = await screen.findByRole('region', { name: 'Kelola portofolio' })
    expect(await within(panel).findByText('1 proyek')).toBeInTheDocument()
    await u.type(within(panel).getByLabelText('Nama portofolio baru'), 'Digital')
    await u.click(within(panel).getByRole('button', { name: 'Tambah' }))
    await waitFor(() => expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ nama: 'Digital' }))
  })
})
