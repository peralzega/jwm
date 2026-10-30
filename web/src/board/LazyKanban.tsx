import { Component, lazy, Suspense, useState, type ComponentProps, type ComponentType, type ReactNode } from 'react'

import { Alert, Button, Loading } from '../components/ui'
import type { KanbanBoard as KanbanBoardType } from './kanban/KanbanBoard'

// Kanban (beserta dnd-kit) dimuat terpisah saat tab Kanban dibuka, agar
// pengguna tabel tidak mengunduhnya.

type KanbanProps = ComponentProps<typeof KanbanBoardType>
export type KanbanLoader = () => Promise<{ default: ComponentType<KanbanProps> }>

const loadKanban: KanbanLoader = () => import('./kanban/KanbanBoard').then((m) => ({ default: m.KanbanBoard }))

class LoadErrorBoundary extends Component<{ onRetry: () => void; children: ReactNode }, { failed: boolean }> {
  state = { failed: false }

  static getDerivedStateFromError(): { failed: boolean } {
    return { failed: true }
  }

  render() {
    if (!this.state.failed) return this.props.children
    return (
      <div className="gap-d flex flex-col items-start">
        <Alert>Tampilan Kanban gagal dimuat. Periksa koneksi Anda, lalu coba lagi.</Alert>
        <Button onClick={this.props.onRetry}>Coba lagi</Button>
      </div>
    )
  }
}

/**
 * Kanban yang dimuat saat dibutuhkan. Bila bagian kode gagal diunduh
 * (jaringan putus, versi baru sudah dipasang), tampil pesan dan tombol
 * "Coba lagi"; React.lazy menyimpan kegagalan, jadi percobaan ulang
 * membuat komponen lazy baru.
 */
export function LazyKanban({ load = loadKanban, ...props }: KanbanProps & { load?: KanbanLoader }) {
  const [attempt, setAttempt] = useState(0)
  // Komponen lazy dibuat sekali di awal dan sekali per "Coba lagi" (di penangan klik), bukan tiap render.
  const [Kanban, setKanban] = useState(() => lazy(load))
  const retry = () => {
    setKanban(() => lazy(load))
    setAttempt((a) => a + 1)
  }
  return (
    <LoadErrorBoundary key={attempt} onRetry={retry}>
      <Suspense fallback={<Loading />}>
        <Kanban {...props} />
      </Suspense>
    </LoadErrorBoundary>
  )
}
