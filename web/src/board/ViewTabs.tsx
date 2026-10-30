import { TabsList, TabsTrigger } from '@/components/ui/tabs'

export type BoardView = 'tabel' | 'kanban' | 'gantt'

const VIEWS: { id: BoardView; label: string }[] = [
  { id: 'tabel', label: 'Tabel' },
  { id: 'kanban', label: 'Kanban' },
  { id: 'gantt', label: 'Gantt' },
]

export function parseView(v: string | null): BoardView {
  return v === 'kanban' || v === 'gantt' ? v : 'tabel'
}

export function isBoardView(v: string): v is BoardView {
  return VIEWS.some((x) => x.id === v)
}

/** Daftar tab tampilan board (shadcn Tabs; panah kiri/kanan berpindah tab). Dipakai di dalam <Tabs>. */
export function ViewTabsList() {
  return (
    <TabsList aria-label="Tampilan board">
      {VIEWS.map((v) => (
        <TabsTrigger key={v.id} value={v.id}>
          {v.label}
        </TabsTrigger>
      ))}
    </TabsList>
  )
}
