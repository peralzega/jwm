import { useDroppable } from '@dnd-kit/core'
import { useVirtualizer } from '@tanstack/react-virtual'
import { memo, useContext, useEffect, useMemo, useRef } from 'react'

import type { Group } from '../../api/types'
import { cssPx, useDensity } from '../../density'
import type { KanbanColumnData } from './columns'
import { KanbanContext } from './context'
import { KanbanCard } from './KanbanCard'

export const KanbanColumn = memo(function KanbanColumn({
  col,
  groups,
}: {
  col: KanbanColumnData
  groups: ReadonlyMap<string, Group>
}) {
  const { setNodeRef, isOver } = useDroppable({ id: col.id })
  const scrollRef = useRef<HTMLDivElement>(null)
  const density = useDensity()
  const slotPx = useMemo(
    () => cssPx('--kanban-card-h', 64) + cssPx('--space-sm', 4),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- dibaca ulang saat kepadatan berganti
    [density],
  )

  const virtualizer = useVirtualizer({
    count: col.items.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => slotPx,
    getItemKey: (i) => col.items[i]?.id ?? i,
    overscan: 4,
  })

  useEffect(() => {
    virtualizer.measure()
  }, [slotPx, virtualizer])

  const { lifted, focusTarget } = useContext(KanbanContext)
  const isTarget = isOver || (lifted !== null && lifted.target === col.id)
  const sectionRef = useRef<HTMLElement | null>(null)

  // Kolom tujuan seret keyboard digulir agar terlihat.
  useEffect(() => {
    if (lifted?.target === col.id) sectionRef.current?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' })
  }, [lifted?.target, col.id])

  // Kartu yang baru dilepas di kolom ini mungkin di luar layar: gulir ke sana agar terpasang dan bisa difokus.
  const focusIndex = focusTarget?.columnId === col.id ? col.items.findIndex((it) => it.id === focusTarget.itemId) : -1
  useEffect(() => {
    if (focusIndex >= 0) virtualizer.scrollToIndex(focusIndex, { align: 'auto' })
  }, [focusIndex, virtualizer])

  const headingId = `kanban-${col.id}`
  return (
    <section
      ref={(el) => {
        setNodeRef(el)
        sectionRef.current = el
      }}
      aria-labelledby={headingId}
      data-kanban-col={col.id}
      className={`flex w-(--kanban-col-w) shrink-0 flex-col rounded border bg-canvas ${
        isTarget ? 'border-primary ring-(length:--focus-ring-w) ring-primary' : 'border-line'
      }`}
    >
      <h2 id={headingId} className="px-d row-h flex shrink-0 items-center gap-(--space-sm) font-semibold">
        {col.warna ? (
          <span className={`tag-${col.warna} tag-fill rounded px-(--space-sm)`}>{col.judul}</span>
        ) : (
          <span className="text-muted-foreground">{col.judul}</span>
        )}
        <span className="font-normal text-muted-foreground" aria-label={`${col.items.length} kartu`}>
          {col.items.length}
        </span>
      </h2>
      <div ref={scrollRef} data-testid={`kanban-scroll-${col.id}`} className="px-d min-h-0 flex-1 overflow-y-auto">
        <div className="relative" style={{ height: virtualizer.getTotalSize() }}>
          {virtualizer.getVirtualItems().map((v) => {
            const item = col.items[v.index]
            if (!item) return null
            return (
              <div key={item.id} className="kanban-slot absolute top-0 left-0 w-full" style={{ transform: `translateY(${v.start}px)` }}>
                <KanbanCard item={item} group={groups.get(item.group_id)} columnId={col.id} />
              </div>
            )
          })}
        </div>
        {col.items.length === 0 && <p className="py-d text-center text-muted-foreground">Tidak ada kartu</p>}
      </div>
    </section>
  )
})
