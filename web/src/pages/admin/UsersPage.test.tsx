import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import type { AdminUser } from '../../api/types'
import { meReply, mockApi, renderApp, user } from '../../test/utils'

const admin = user({ id: 'admin-1', nama: 'Admin', peran: 'admin' })

function adminUser(over: Partial<AdminUser> = {}): AdminUser {
  return {
    id: 'u-2',
    nama: 'Siti',
    email: 'siti@jiep.co.id',
    peran: 'viewer',
    aktif: true,
    wajib_ganti_sandi: true,
    terkunci_hingga: null,
    login_terakhir: null,
    dibuat_pada: '2026-10-29T00:00:00Z',
    ...over,
  }
}

describe('halaman admin pengguna', () => {
  it('menampilkan sandi sementara satu kali setelah akun dibuat', async () => {
    const calls = mockApi({
      'GET /auth/me': meReply(admin),
      'GET /admin/users': { status: 200, body: { items: [adminUser({ id: 'admin-1', nama: 'Admin', peran: 'admin' })] } },
      'POST /admin/users': { status: 201, body: { user: adminUser(), sandi_sementara: 'ABCDEFGHIJKLMNOPQRSTUVWXYZ' } },
    })
    renderApp('/admin/pengguna')
    const u = userEvent.setup()
    const form = await screen.findByRole('form', { name: 'Buat akun' })
    await u.type(within(form).getByLabelText('Nama'), 'Siti')
    await u.type(within(form).getByLabelText('Email'), 'siti@jiep.co.id')
    await u.click(within(form).getByRole('button', { name: 'Buat akun' }))

    expect(await screen.findByLabelText('Sandi sementara')).toHaveTextContent('ABCDEFGHIJKLMNOPQRSTUVWXYZ')
    expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ nama: 'Siti', email: 'siti@jiep.co.id', peran: 'viewer' })

    await u.click(screen.getByRole('button', { name: 'Tutup' }))
    expect(screen.queryByLabelText('Sandi sementara')).not.toBeInTheDocument()
  })

  it('menampilkan galat dari server, mis. email sudah dipakai', async () => {
    mockApi({
      'GET /auth/me': meReply(admin),
      'GET /admin/users': { status: 200, body: { items: [] } },
      'POST /admin/users': { status: 409, body: { error: 'email_taken', message: 'email sudah dipakai akun lain' } },
    })
    renderApp('/admin/pengguna')
    const u = userEvent.setup()
    const form = await screen.findByRole('form', { name: 'Buat akun' })
    await u.type(within(form).getByLabelText('Nama'), 'Siti')
    await u.type(within(form).getByLabelText('Email'), 'siti@jiep.co.id')
    await u.click(within(form).getByRole('button', { name: 'Buat akun' }))
    expect(await within(form).findByRole('alert')).toHaveTextContent('email sudah dipakai')
  })

  it('mengubah peran dan menonaktifkan akun lewat API admin; akun sendiri tidak bisa dinonaktifkan', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const calls = mockApi({
      'GET /auth/me': meReply(admin),
      'GET /admin/users': {
        status: 200,
        body: { items: [adminUser({ id: 'admin-1', nama: 'Admin', peran: 'admin' }), adminUser()] },
      },
      'PATCH /admin/users/u-2': { status: 200, body: adminUser({ peran: 'member' }) },
      'POST /admin/users/u-2/deactivate': { status: 200, body: adminUser({ aktif: false }) },
    })
    renderApp('/admin/pengguna')
    const u = userEvent.setup()
    await u.selectOptions(await screen.findByLabelText('Peran Siti'), 'member')
    await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ peran: 'member' }))

    const rows = screen.getAllByRole('row')
    const selfRow = rows.find((r) => within(r).queryByText('Admin', { selector: 'td' }))
    expect(within(selfRow as HTMLElement).getByRole('button', { name: 'Nonaktifkan' })).toBeDisabled()

    const sitiRow = rows.find((r) => within(r).queryByText('Siti', { selector: 'td' }))
    await u.click(within(sitiRow as HTMLElement).getByRole('button', { name: 'Nonaktifkan' }))
    await waitFor(() => expect(calls.some((c) => c.path === '/admin/users/u-2/deactivate')).toBe(true))
  })

  it('pengguna non-admin diarahkan dari halaman admin', async () => {
    mockApi({ 'GET /auth/me': meReply(user()), 'GET /projects': { status: 200, body: { items: [] } } })
    const router = renderApp('/admin/pengguna')
    await waitFor(() => expect(router.state.location.pathname).toBe('/'))
    expect(screen.queryByRole('link', { name: 'Pengguna' })).not.toBeInTheDocument()
  })
})
