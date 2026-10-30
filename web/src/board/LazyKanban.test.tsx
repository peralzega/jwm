import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { LazyKanban, type KanbanLoader } from './LazyKanban'
import { makeBoard } from './testData'
import type { ItemMutations } from './useBoardItems'

const board = makeBoard()
const firstCol = board.kolom[0]
if (!firstCol) throw new Error('fixture tanpa kolom status')
const statusCol = firstCol
// Stub tidak memakai mutasi; objek kosong cukup untuk memenuhi tipe props.
const mutations = {} as ItemMutations

function Stub() {
  return <p>Kanban termuat</p>
}

function renderLazy(load: KanbanLoader) {
  render(<LazyKanban load={load} board={board} statusCol={statusCol} items={[]} members={[]} mutations={mutations} />)
}

describe('LazyKanban', () => {
  it('menampilkan status memuat lalu Kanban', async () => {
    let resolve: (m: Awaited<ReturnType<KanbanLoader>>) => void = () => undefined
    renderLazy(() => new Promise((r) => (resolve = r)))
    expect(await screen.findByRole('status')).toHaveTextContent('Memuat')
    resolve({ default: Stub })
    expect(await screen.findByText('Kanban termuat')).toBeInTheDocument()
  })

  it('gagal dimuat: pesan jelas dan "Coba lagi" memuat ulang', async () => {
    // React mencatat galat yang ditangkap error boundary ke console.error.
    vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const load = vi
      .fn<KanbanLoader>()
      .mockRejectedValueOnce(new TypeError('Failed to fetch dynamically imported module'))
      .mockResolvedValueOnce({ default: Stub })
    renderLazy(load)

    expect(await screen.findByRole('alert')).toHaveTextContent('Tampilan Kanban gagal dimuat. Periksa koneksi Anda, lalu coba lagi.')
    expect(screen.queryByText('Kanban termuat')).not.toBeInTheDocument()

    await userEvent.setup().click(screen.getByRole('button', { name: 'Coba lagi' }))
    expect(await screen.findByText('Kanban termuat')).toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(load).toHaveBeenCalledTimes(2)
  })

  it('gagal lagi setelah "Coba lagi": pesan tetap tampil', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const load = vi.fn<KanbanLoader>().mockRejectedValue(new TypeError('offline'))
    renderLazy(load)
    await userEvent.setup().click(await screen.findByRole('button', { name: 'Coba lagi' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('gagal dimuat')
    expect(load).toHaveBeenCalledTimes(2)
  })
})
