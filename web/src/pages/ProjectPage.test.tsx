import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import type { BudgetEntry, Project } from '../api/types'
import { meReply, mockApi, renderApp, user } from '../test/utils'

const project = (over: Partial<Project> = {}): Project => ({
  id: 'p1',
  nama: 'Jaringan',
  pemilik_id: 'u-1',
  pemilik_nama: 'Budi',
  mulai: null,
  selesai: null,
  prioritas: 'sedang',
  dibuat_pada: '2026-10-29T00:00:00Z',
  diubah_pada: '2026-10-29T00:00:00Z',
  diarsipkan_pada: null,
  peran_saya: 'manager',
  portfolio_id: null,
  portfolio_nama: null,
  ...over,
})

describe('pengaturan proyek', () => {
  it('Project Manager memilih portofolio proyek', async () => {
    const calls = mockApi({
      'GET /auth/me': meReply(user({ peran: 'project_manager' })),
      'GET /projects/p1': { status: 200, body: project() },
      'GET /projects/p1/boards': { status: 200, body: { items: [] } },
      'GET /projects/p1/members': { status: 200, body: { items: [] } },
      'GET /projects/p1/budget': { status: 200, body: { items: [], total: { anggaran: '0', realisasi: '0', sisa: '0' }, bisa_ubah: false } },
      'GET /portfolios': {
        status: 200,
        body: { items: [{ id: 'pf1', nama: 'Infrastruktur', jumlah_proyek: 0, dibuat_pada: '', diubah_pada: '' }] },
      },
      'PATCH /projects/p1': (c) => ({
        status: 200,
        body: project({ portfolio_id: (c.body as { portfolio_id: string }).portfolio_id, portfolio_nama: 'Infrastruktur', diubah_pada: 'baru' }),
      }),
    })
    renderApp('/proyek/p1')
    const u = userEvent.setup()
    const form = await screen.findByRole('form', { name: 'Ubah proyek' })
    const select = within(form).getByLabelText('Portofolio')
    await within(select).findByRole('option', { name: 'Infrastruktur' })
    expect(select).toHaveValue('')
    await u.selectOptions(select, 'pf1')
    await u.click(within(form).getByRole('button', { name: 'Simpan' }))
    await waitFor(() =>
      expect(calls.find((c) => c.method === 'PATCH')?.body).toMatchObject({ portfolio_id: 'pf1', prioritas: 'sedang' }),
    )
    expect(await screen.findByText(/portofolio Infrastruktur/)).toBeInTheDocument()
  })
})

describe('keuangan proyek', () => {
  const entry = (over: Partial<BudgetEntry>): BudgetEntry => ({
    id: 'e1',
    project_id: 'p1',
    tanggal: '2026-11-02',
    jenis: 'anggaran',
    jumlah: '1500000.00',
    keterangan: 'Anggaran awal',
    dibuat_oleh_nama: 'Budi',
    dibuat_pada: '',
    diubah_pada: '',
    ...over,
  })

  function setup(bisaUbah: boolean) {
    let items = [entry({})]
    const calls = mockApi({
      'GET /auth/me': meReply(user({ peran: 'project_manager' })),
      'GET /projects/p1': { status: 200, body: project({ peran_saya: bisaUbah ? 'manager' : 'member' }) },
      'GET /projects/p1/boards': { status: 200, body: { items: [] } },
      'GET /projects/p1/members': { status: 200, body: { items: [] } },
      'GET /portfolios': { status: 200, body: { items: [] } },
      'GET /projects/p1/budget': () => ({
        status: 200,
        body: {
          items,
          total: { anggaran: '1500000.00', realisasi: items.length > 1 ? '1600000.50' : '0.00', sisa: items.length > 1 ? '-100000.50' : '1500000.00' },
          bisa_ubah: bisaUbah,
        },
      }),
      'POST /projects/p1/budget': (c) => {
        items = [entry({ id: 'e2', jenis: 'realisasi', jumlah: (c.body as { jumlah: string }).jumlah }), ...items]
        return { status: 201, body: { id: 'e2' } }
      },
      'DELETE /budget-entries/e1': () => {
        items = items.filter((e) => e.id !== 'e1')
        return { status: 204 }
      },
    })
    renderApp('/proyek/p1')
    return calls
  }

  it('PMO/Project Manager menambah dan menghapus entri; sisa negatif ditandai', async () => {
    const calls = setup(true)
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const u = userEvent.setup()
    const panel = await screen.findByRole('region', { name: 'Keuangan' })
    expect(await within(panel).findByLabelText('Total keuangan')).toHaveTextContent(/Anggaran\s*Rp\s1\.500\.000/)
    const form = within(panel).getByRole('form', { name: 'Tambah entri keuangan' })
    const add = within(form).getByRole('button', { name: 'Tambah' })
    await u.type(within(form).getByLabelText('Jumlah (Rp)'), '1.600.000')
    expect(add).toBeDisabled()
    await u.clear(within(form).getByLabelText('Jumlah (Rp)'))
    await u.type(within(form).getByLabelText('Jumlah (Rp)'), '1600000,50')
    await u.type(within(form).getByLabelText('Keterangan'), 'Pembayaran vendor')
    await u.click(add)
    await waitFor(() =>
      expect(calls.find((c) => c.method === 'POST')?.body).toMatchObject({ jenis: 'realisasi', jumlah: '1600000.50', keterangan: 'Pembayaran vendor' }),
    )
    await waitFor(() => expect(within(panel).getByText(/-Rp\s100\.000,5/)).toHaveClass('text-danger'))
    await u.click(within(panel).getByRole('button', { name: 'Hapus entri 2 Nov 2026 Anggaran' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'DELETE')).toBe(true))
  })

  it('anggota lain hanya melihat', async () => {
    setup(false)
    const panel = await screen.findByRole('region', { name: 'Keuangan' })
    expect(await within(panel).findByText('Anggaran awal')).toBeInTheDocument()
    expect(within(panel).queryByRole('form')).not.toBeInTheDocument()
    expect(within(panel).queryByRole('button', { name: /Hapus/ })).not.toBeInTheDocument()
  })
})
