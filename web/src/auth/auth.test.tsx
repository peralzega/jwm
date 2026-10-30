import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'

import { validateNewPassword } from '../pages/ChangePasswordPage'
import { meReply, mockApi, renderApp, user } from '../test/utils'

describe('alur autentikasi', () => {
  it('mengarahkan pengguna yang belum login ke /login', async () => {
    mockApi({ 'GET /auth/me': meReply(null) })
    const router = renderApp('/proyek/abc')
    expect(await screen.findByRole('heading', { name: 'Masuk ke JWM' })).toBeInTheDocument()
    expect(router.state.location.pathname).toBe('/login')
  })

  it('menampilkan pesan galat seragam dari server dan mengosongkan kolom sandi', async () => {
    mockApi({
      'GET /auth/me': meReply(null),
      'POST /auth/login': { status: 401, body: { error: 'invalid_credentials', message: 'Email atau kata sandi salah.' } },
    })
    renderApp('/login')
    const u = userEvent.setup()
    await u.type(await screen.findByLabelText('Email'), 'budi@jiep.co.id')
    await u.type(screen.getByLabelText('Kata sandi'), 'salah-sandi-panjang')
    await u.click(screen.getByRole('button', { name: 'Masuk' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('Email atau kata sandi salah.')
    expect(screen.getByLabelText('Kata sandi')).toHaveValue('')
  })

  it('setelah login dengan sandi sementara, pengguna diarahkan ke ganti sandi dan tidak bisa membuka halaman lain', async () => {
    const temp = user({ wajib_ganti_sandi: true })
    mockApi({
      'GET /auth/me': meReply(null),
      'POST /auth/login': { status: 200, body: { user: temp, csrf_token: 't' } },
    })
    const router = renderApp('/login')
    const u = userEvent.setup()
    await u.type(await screen.findByLabelText('Email'), 'budi@jiep.co.id')
    await u.type(screen.getByLabelText('Kata sandi'), 'sandi-sementara-01')
    await u.click(screen.getByRole('button', { name: 'Masuk' }))

    expect(await screen.findByRole('heading', { name: 'Ganti kata sandi' })).toBeInTheDocument()
    await router.navigate('/')
    await waitFor(() => expect(router.state.location.pathname).toBe('/ganti-sandi'))
  })

  it('validasi ganti sandi di klien mencegah permintaan yang pasti ditolak', async () => {
    expect(validateNewPassword('lama-sekali-01', 'pendek', 'pendek')).toMatch(/minimal 12/)
    expect(validateNewPassword('lama-sekali-01', 'baru-yang-panjang', 'baru-yang-beda')).toMatch(/tidak sama/)
    expect(validateNewPassword('sama-sama-panjang', 'sama-sama-panjang', 'sama-sama-panjang')).toMatch(/berbeda/)
    expect(validateNewPassword('lama-sekali-01', 'baru-yang-panjang', 'baru-yang-panjang')).toBeNull()

    const calls = mockApi({ 'GET /auth/me': meReply(user({ wajib_ganti_sandi: true })) })
    renderApp('/ganti-sandi')
    const u = userEvent.setup()
    await u.type(await screen.findByLabelText('Kata sandi saat ini'), 'sandi-sementara-01')
    await u.type(screen.getByLabelText('Kata sandi baru'), 'pendek')
    await u.type(screen.getByLabelText('Ulangi kata sandi baru'), 'pendek')
    await u.click(screen.getByRole('button', { name: 'Simpan' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('minimal 12')
    expect(calls.some((c) => c.path === '/auth/password')).toBe(false)
  })

  it('ganti sandi berhasil membawa pengguna ke daftar proyek', async () => {
    const calls = mockApi({
      'GET /auth/me': meReply(user({ wajib_ganti_sandi: true })),
      'POST /auth/password': { status: 200, body: { user: user(), csrf_token: 'baru' } },
      'GET /projects': { status: 200, body: { items: [] } },
    })
    const router = renderApp('/ganti-sandi')
    const u = userEvent.setup()
    await u.type(await screen.findByLabelText('Kata sandi saat ini'), 'sandi-sementara-01')
    await u.type(screen.getByLabelText('Kata sandi baru'), 'kuda-lari-di-sawah')
    await u.type(screen.getByLabelText('Ulangi kata sandi baru'), 'kuda-lari-di-sawah')
    await u.click(screen.getByRole('button', { name: 'Simpan' }))

    await waitFor(() => expect(router.state.location.pathname).toBe('/'))
    expect(calls.find((c) => c.path === '/auth/password')?.body).toEqual({
      sandi_lama: 'sandi-sementara-01',
      sandi_baru: 'kuda-lari-di-sawah',
    })
  })
})
