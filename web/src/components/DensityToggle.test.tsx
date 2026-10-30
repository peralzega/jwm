import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it } from 'vitest'

import { meReply, mockApi, renderApp, user } from '../test/utils'

beforeEach(() => {
  document.documentElement.dataset.density = 'compact'
})

describe('pilihan kepadatan', () => {
  it('memakai kepadatan dari server dan menyimpan perubahan', async () => {
    const calls = mockApi({
      'GET /auth/me': meReply(user({ kepadatan: 'normal' })),
      'GET /projects': { status: 200, body: { items: [] } },
      'PATCH /auth/me/preferensi': { status: 200, body: { user: user({ kepadatan: 'compact' }), csrf_token: 't' } },
    })
    renderApp('/')
    const normal = await screen.findByRole('button', { name: 'Normal' })
    await waitFor(() => expect(document.documentElement.dataset.density).toBe('normal'))
    expect(normal).toHaveAttribute('aria-pressed', 'true')

    await userEvent.setup().click(screen.getByRole('button', { name: 'Ringkas' }))
    await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ kepadatan: 'compact' }))
    expect(document.documentElement.dataset.density).toBe('compact')
    expect(screen.getByRole('button', { name: 'Ringkas' })).toHaveAttribute('aria-pressed', 'true')
  })

  it('dikembalikan bila server menolak', async () => {
    mockApi({
      'GET /auth/me': meReply(user({ kepadatan: 'compact' })),
      'GET /projects': { status: 200, body: { items: [] } },
      'PATCH /auth/me/preferensi': { status: 500, body: { error: 'internal', message: 'Terjadi kesalahan internal.' } },
    })
    renderApp('/')
    await userEvent.setup().click(await screen.findByRole('button', { name: 'Normal' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Terjadi kesalahan internal.')
    expect(document.documentElement.dataset.density).toBe('compact')
    expect(screen.getByRole('button', { name: 'Ringkas' })).toHaveAttribute('aria-pressed', 'true')
  })
})
