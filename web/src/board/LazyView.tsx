import { Component, lazy, Suspense, useState, type ComponentType, type ReactNode } from 'react'

import { Alert, Button, Loading } from '../components/ui'

// Tampilan board yang berat (Kanban dengan dnd-kit, Gantt) dimuat terpisah
// saat tabnya dibuka, agar pengguna tabel tidak mengunduhnya.

export type ViewLoader<P> = () => Promise<{ default: ComponentType<P> }>

class LoadErrorBoundary extends Component<{ name: string; onRetry: () => void; children: ReactNode }, { failed: boolean }> {
  state = { failed: false }

  static getDerivedStateFromError(): { failed: boolean } {
    return { failed: true }
  }

  render() {
    if (!this.state.failed) return this.props.children
    return (
      <div className="gap-d flex flex-col items-start">
        <Alert>Tampilan {this.props.name} gagal dimuat. Periksa koneksi Anda, lalu coba lagi.</Alert>
        <Button onClick={this.props.onRetry}>Coba lagi</Button>
      </div>
    )
  }
}

/**
 * Komponen yang dimuat saat dibutuhkan. Bila bagian kode gagal diunduh
 * (jaringan putus, versi baru sudah dipasang), tampil pesan dan tombol
 * "Coba lagi"; React.lazy menyimpan kegagalan, jadi percobaan ulang
 * membuat komponen lazy baru.
 */
export function LazyView<P extends object>({ name, load, props }: { name: string; load: ViewLoader<P>; props: P }) {
  const [attempt, setAttempt] = useState(0)
  // Komponen lazy dibuat sekali di awal dan sekali per "Coba lagi" (di penangan klik), bukan tiap render.
  const [View, setView] = useState(() => lazy(load))
  const retry = () => {
    setView(() => lazy(load))
    setAttempt((a) => a + 1)
  }
  return (
    <LoadErrorBoundary key={attempt} name={name} onRetry={retry}>
      <Suspense fallback={<Loading />}>
        <View {...props} />
      </Suspense>
    </LoadErrorBoundary>
  )
}
