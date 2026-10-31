import { createContext, useContext, useMemo, type ReactNode } from 'react'

import type { Dependency, Item } from '../../api/types'
import { useDependencies } from './queries'

export interface DependencyLinkView {
  dep: Dependency
  /** Item di ujung lain (predecessor atau successor). */
  other: Item | undefined
}

interface DepsValue {
  boardId: string
  deps: Dependency[]
  items: Item[]
  byId: ReadonlyMap<string, Item>
  predecessors: ReadonlyMap<string, DependencyLinkView[]>
  successors: ReadonlyMap<string, DependencyLinkView[]>
  loadError: unknown
}

const DepsContext = createContext<DepsValue | null>(null)

/**
 * Dependensi board untuk sel tabel, Gantt, dan editor dependensi.
 * enabled = false: dependensi tidak dimuat (board tanpa kolom dependency, bukan tampilan Gantt).
 */
export function DependencyProvider({
  boardId,
  items,
  enabled,
  children,
}: {
  boardId: string
  items: Item[]
  enabled: boolean
  children: ReactNode
}) {
  const { deps, error } = useDependencies(boardId, enabled)
  const value = useMemo(() => {
    const byId = new Map(items.map((it) => [it.id, it]))
    const predecessors = new Map<string, DependencyLinkView[]>()
    const successors = new Map<string, DependencyLinkView[]>()
    for (const dep of deps) {
      const p = predecessors.get(dep.successor_id) ?? []
      p.push({ dep, other: byId.get(dep.predecessor_id) })
      predecessors.set(dep.successor_id, p)
      const s = successors.get(dep.predecessor_id) ?? []
      s.push({ dep, other: byId.get(dep.successor_id) })
      successors.set(dep.predecessor_id, s)
    }
    return { boardId, deps, items, byId, predecessors, successors, loadError: error }
  }, [boardId, items, deps, error])
  return <DepsContext.Provider value={value}>{children}</DepsContext.Provider>
}

export function useDepsContext(): DepsValue {
  const v = useContext(DepsContext)
  if (!v) throw new Error('DependencyProvider belum dipasang')
  return v
}

/** Judul predecessor item, mis. "Desain, Pengadaan". */
export function predecessorText(v: DepsValue, itemId: string): string {
  return (v.predecessors.get(itemId) ?? []).map((l) => l.other?.judul ?? 'Item terhapus').join(', ')
}

/** Isi sel kolom dependency: daftar predecessor. */
export function DependencyCellText({ item }: { item: Item }) {
  const v = useContext(DepsContext)
  const text = v ? predecessorText(v, item.id) : ''
  return (
    <span className="px-d flex h-full items-center truncate" title={text || undefined}>
      {text || <span className="text-muted-foreground">—</span>}
    </span>
  )
}
