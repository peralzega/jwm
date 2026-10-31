import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import type { CalendarDay, CalendarYear } from '../../api/types'
import { meReply, mockApi, renderApp, user } from '../../test/utils'

const admin = user({ id: 'admin-1', nama: 'Admin', peran: 'admin' })
const THIS_YEAR = new Date().getFullYear()

function yearReply(items: CalendarDay[], terisi: number[]): { status: number; body: CalendarYear } {
  return { status: 200, body: { tahun: THIS_YEAR, items, tahun_terisi: terisi } }
}

describe('halaman admin kalender kerja', () => {
  it('peringatan bila tahun belum diisi; simpan satu tanggal', async () => {
    let items: CalendarDay[] = []
    const calls = mockApi({
      'GET /auth/me': meReply(admin),
      'GET /work-calendar': () => yearReply(items, items.length ? [THIS_YEAR] : []),
      [`PUT /admin/work-calendar/${THIS_YEAR}-12-25`]: (c) => {
        const b = c.body as { hari_kerja: boolean; keterangan: string }
        items = [{ tanggal: `${THIS_YEAR}-12-25`, ...b }]
        return { status: 200, body: items[0] }
      },
    })
    renderApp('/admin/kalender')
    const u = userEvent.setup()
    expect(await screen.findByText(`Kalender ${THIS_YEAR} belum diisi`, { exact: false })).toBeInTheDocument()

    const form = screen.getByRole('form', { name: 'Tambah atau ubah tanggal' })
    fireEvent.change(within(form).getByLabelText('Tanggal'), { target: { value: `${THIS_YEAR}-12-25` } })
    await u.type(within(form).getByLabelText('Keterangan'), 'Natal')
    await u.click(within(form).getByRole('button', { name: 'Simpan tanggal' }))
    await waitFor(() =>
      expect(calls.find((c) => c.method === 'PUT')?.body).toEqual({ hari_kerja: false, keterangan: 'Natal' }),
    )
    const table = await screen.findByRole('table')
    expect(within(table).getByText('Natal')).toBeInTheDocument()
    expect(within(table).getByText('Libur')).toBeInTheDocument()
    expect(screen.queryByText(`Kalender ${THIS_YEAR} belum diisi`, { exact: false })).not.toBeInTheDocument()
  })

  it('impor CSV: isi berkas dikirim apa adanya, galat per baris ditampilkan', async () => {
    const calls = mockApi({
      'GET /auth/me': meReply(admin),
      'GET /work-calendar': yearReply([], []),
      'POST /admin/work-calendar/import': (c) =>
        (c.body as { csv: string }).csv.includes('salah')
          ? { status: 422, body: { error: 'invalid_csv', message: 'berkas CSV kalender tidak valid: baris 2: tanggal "salah" harus berformat YYYY-MM-DD' } }
          : { status: 200, body: { jumlah: 2, tahun: [2027], dihapus: 0 } },
    })
    renderApp('/admin/kalender')
    const u = userEvent.setup()
    const form = await screen.findByRole('form', { name: 'Impor CSV' })
    const input = within(form).getByLabelText('Berkas CSV')

    await u.upload(input, new File(['tanggal,keterangan\nsalah,x\n'], 'libur.csv', { type: 'text/csv' }))
    await u.click(within(form).getByRole('button', { name: 'Impor' }))
    expect(await within(form).findByRole('alert')).toHaveTextContent('baris 2')

    const csv = 'tanggal,keterangan\n2027-01-01,Tahun Baru\n2027-12-25,Natal\n'
    await u.upload(input, new File([csv], 'libur.csv', { type: 'text/csv' }))
    await u.click(within(form).getByLabelText('Ganti seluruh isi tahun yang ada di berkas'))
    await u.click(within(form).getByRole('button', { name: 'Impor' }))
    expect(await within(form).findByText('2 tanggal diimpor untuk tahun 2027.')).toBeInTheDocument()
    expect(calls.filter((c) => c.method === 'POST').at(-1)?.body).toEqual({ csv, ganti: true })
  })

  it('menghapus tanggal setelah konfirmasi', async () => {
    let items: CalendarDay[] = [{ tanggal: `${THIS_YEAR}-08-17`, hari_kerja: false, keterangan: 'HUT RI' }]
    mockApi({
      'GET /auth/me': meReply(admin),
      'GET /work-calendar': () => yearReply(items, [THIS_YEAR]),
      [`DELETE /admin/work-calendar/${THIS_YEAR}-08-17`]: () => {
        items = []
        return { status: 204 }
      },
    })
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    renderApp('/admin/kalender')
    const u = userEvent.setup()
    const del = await screen.findByRole('button', { name: /^Hapus .*17 Agustus/ })
    await u.click(del)
    await waitFor(() => expect(screen.queryByText('HUT RI')).not.toBeInTheDocument())
  })

  it('bukan Admin tidak dapat membuka halaman', async () => {
    mockApi({ 'GET /auth/me': meReply(user({ peran: 'pmo' })), 'GET /projects': { status: 200, body: { items: [] } } })
    renderApp('/admin/kalender')
    await waitFor(() => expect(screen.queryByRole('heading', { name: 'Kalender kerja' })).not.toBeInTheDocument())
    expect(screen.queryByRole('link', { name: 'Kalender kerja' })).not.toBeInTheDocument()
  })
})
