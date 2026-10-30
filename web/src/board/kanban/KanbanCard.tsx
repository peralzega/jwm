import { useDraggable } from '@dnd-kit/core'
import { memo, useContext, useEffect } from 'react'

import type { Column, Group, Item } from '../../api/types'
import { asString, asStringArray, formatDate, initials, isPriority, PRIORITY_STYLE } from '../format'
import { KanbanContext } from './context'

/** Kolom board yang ditampilkan di kartu (yang pertama dari tiap tipe). */
export interface CardFields {
  person?: Column
  priority?: Column
  date?: Column
}

/** Isi kartu tanpa perilaku seret; dipakai kartu asli dan salinan saat diseret. */
export function CardBody({ item, group }: { item: Item; group: Group | undefined }) {
  const { fields, names } = useContext(KanbanContext)
  const people = fields.person ? asStringArray(item.nilai[fields.person.id]) : []
  const prio = fields.priority ? item.nilai[fields.priority.id] : undefined
  const date = fields.date ? formatDate(asString(item.nilai[fields.date.id]), true) : ''
  const names_ = people.map((id) => names.get(id) ?? 'Pengguna')

  return (
    <div className="kanban-card px-d py-d flex flex-col justify-between gap-(--space-sm) rounded border border-line bg-surface">
      <div className="flex min-w-0 items-center gap-(--space-sm)">
        {group && (
          <span
            aria-hidden="true"
            title={group.judul}
            className={`tag-${group.warna} tag-dot size-(--icon-size) shrink-0 rounded-full`}
          />
        )}
        <span className="min-w-0 flex-1 truncate font-medium" title={item.judul} data-testid="card-title">
          {item.judul}
        </span>
      </div>
      <div className="flex min-w-0 items-center gap-(--space-sm) text-(length:--font-xs)">
        <span className="flex shrink-0 items-center gap-(--space-xs)" title={names_.join(', ')}>
          {names_.slice(0, 3).map((nama, i) => (
            <span
              key={people[i]}
              aria-hidden="true"
              className="inline-flex size-(--avatar-size) items-center justify-center rounded-full bg-accent font-semibold text-white"
            >
              {initials(nama)}
            </span>
          ))}
          {names_.length > 0 && <span className="sr-only">{names_.join(', ')}</span>}
        </span>
        {isPriority(prio) && (
          <span className={`tag-${PRIORITY_STYLE[prio].warna} tag-fill rounded px-(--space-sm)`}>
            {PRIORITY_STYLE[prio].judul}
          </span>
        )}
        {date && <span className="ml-auto shrink-0 text-muted">{date}</span>}
      </div>
    </div>
  )
}

/** Kartu yang dapat diseret (bila item boleh diubah). */
export const KanbanCard = memo(function KanbanCard({
  item,
  group,
  columnId,
}: {
  item: Item
  group: Group | undefined
  columnId: string
}) {
  const { lifted, focusTarget, onCardKey, onFocused } = useContext(KanbanContext)
  const { attributes, listeners, setNodeRef, node, isDragging } = useDraggable({
    id: item.id,
    data: { columnId },
    disabled: !item.bisa_ubah,
  })
  const isLifted = lifted?.itemId === item.id

  // Setelah dilepas, kartu dipasang ulang di kolom tujuan; fokus ikut pindah. Kartu lama di
  // kolom asal (pembaruan optimistic belum diterapkan) tidak boleh mengambil fokus.
  const shouldFocus = focusTarget?.itemId === item.id && focusTarget.columnId === columnId
  useEffect(() => {
    if (!shouldFocus || !node.current) return
    node.current.focus({ preventScroll: true })
    onFocused()
  }, [shouldFocus, node, onFocused])

  return (
    <div
      ref={setNodeRef}
      {...attributes}
      {...listeners}
      onKeyDown={(e) => onCardKey(e, item, columnId)}
      aria-roledescription="kartu yang dapat diseret"
      aria-label={`${item.judul}${item.bisa_ubah ? '' : ' (hanya baca)'}`}
      aria-pressed={item.bisa_ubah ? isLifted : undefined}
      data-kanban-card={item.id}
      className={`rounded outline-none focus-visible:ring-(length:--focus-ring-w) focus-visible:ring-accent ${
        // touch-manipulation: sentuhan singkat tetap menggulir kolom; seret
        // dimulai lewat tahan sebentar (TouchSensor dengan jeda).
        item.bisa_ubah ? 'cursor-grab touch-manipulation' : 'cursor-default'
      } ${isDragging || isLifted ? 'opacity-40' : ''}`}
    >
      <CardBody item={item} group={group} />
    </div>
  )
})
