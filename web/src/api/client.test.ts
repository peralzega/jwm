import { describe, expect, it, vi } from 'vitest'

import { mockApi } from '../test/utils'
import { api, ApiError, errorMessage } from './client'

describe('api', () => {
  it('menyimpan token CSRF dari respons dan mengirimnya hanya pada metode pengubah data', async () => {
    const calls = mockApi({
      'POST /auth/login': { status: 200, body: { user: {}, csrf_token: 'tok-1' } },
      'GET /projects': { status: 200, body: { items: [] } },
      'PATCH /projects/p1': { status: 200, body: {} },
    })
    await api('POST', '/auth/login', { email: 'a', password: 'b' })
    await api('GET', '/projects')
    await api('PATCH', '/projects/p1', { nama: 'x' })

    expect(calls[0]?.headers['X-CSRF-Token']).toBeUndefined()
    expect(calls[1]?.headers['X-CSRF-Token']).toBeUndefined()
    expect(calls[2]?.headers['X-CSRF-Token']).toBe('tok-1')
    expect(calls[2]?.body).toEqual({ nama: 'x' })
  })

  it('mengubah galat API menjadi ApiError dengan kode dan pesan server', async () => {
    mockApi({ 'POST /auth/login': { status: 401, body: { error: 'invalid_credentials', message: 'Email atau kata sandi salah.' } } })
    const err = await api('POST', '/auth/login', {}).catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err).toMatchObject({ status: 401, code: 'invalid_credentials' })
    expect(errorMessage(err)).toBe('Email atau kata sandi salah.')
  })

  it('204 menghasilkan undefined; galat jaringan menjadi kode network', async () => {
    mockApi({ 'DELETE /items/i1': { status: 204 } })
    await expect(api('DELETE', '/items/i1')).resolves.toBeUndefined()

    vi.stubGlobal('fetch', vi.fn(async () => Promise.reject(new TypeError('offline'))))
    await expect(api('GET', '/projects')).rejects.toMatchObject({ code: 'network' })
  })
})
