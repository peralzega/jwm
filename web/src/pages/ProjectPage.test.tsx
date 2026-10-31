import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'

import type { Project } from '../api/types'
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
