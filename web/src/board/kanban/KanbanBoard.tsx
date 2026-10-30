import {
  DndContext,
  DragOverlay,
  MouseSensor,
  TouchSensor,
  useSensor,
  useSensors,
  type Announcements,
  type DragEndEvent,
  type DragStartEvent,
  type UniqueIdentifier,
} from '@dnd-kit/core'
import { useCallback, useMemo, useState, type KeyboardEvent } from 'react'

import type { BoardDetail, Column, Item, Member } from '../../api/types'
import type { ItemMutations } from '../useBoardItems'
import { groupByStatus, statusValueFor } from './columns'
import { KanbanContext, type KanbanContextValue } from './context'
import { CardBody, type CardFields } from './KanbanCard'
import { KanbanColumn } from './KanbanColumn'
import { kanbanCollision, keyAction, type Lifted } from './keyboard'

// Ambang aktivasi seret (perilaku, bukan ukuran tampilan):
// mouse mulai menyeret setelah bergeser beberapa piksel agar klik tidak memicu seret;
// sentuhan perlu ditahan sebentar agar geser biasa tetap menggulir kolom.
const MOUSE_DISTANCE = 4
const TOUCH_DELAY_MS = 200
const TOUCH_TOLERANCE = 6

const INSTRUCTIONS =
  'Tekan Spasi atau Enter untuk mengangkat kartu. Gunakan panah kiri dan kanan untuk memilih kolom, lalu Spasi atau Enter untuk melepas. Esc membatalkan.'

function firstOf(kolom: Column[], tipe: Column['tipe']): Column | undefined {
  return [...kolom].sort((a, b) => a.urutan - b.urutan).find((c) => c.tipe === tipe)
}

export function KanbanBoard({
  board,
  statusCol,
  items,
  members,
  mutations,
}: {
  board: BoardDetail
  statusCol: Column
  items: Item[]
  members: Member[]
  mutations: ItemMutations
}) {
  const [activeId, setActiveId] = useState<UniqueIdentifier | null>(null)
  const [lifted, setLifted] = useState<Lifted | null>(null)
  const [focusTarget, setFocusTarget] = useState<{ itemId: string; columnId: string } | null>(null)
  const [message, setMessage] = useState('')

  const columns = useMemo(() => groupByStatus(statusCol, board.grup, items), [statusCol, board.grup, items])
  const columnIds = useMemo(() => columns.map((c) => c.id), [columns])
  const groups = useMemo(() => new Map(board.grup.map((g) => [g.id, g])), [board.grup])
  const itemsById = useMemo(() => new Map(items.map((it) => [it.id, it])), [items])
  const colName = useCallback((id: UniqueIdentifier | undefined) => columns.find((c) => c.id === id)?.judul ?? '', [columns])
  const title = (id: UniqueIdentifier) => itemsById.get(String(id))?.judul ?? 'Kartu'

  const moveCard = useCallback(
    (item: Item, from: string, to: string) => {
      if (to === from || !item.bisa_ubah) return false
      mutations.setValue.mutate({ itemId: item.id, columnId: statusCol.id, nilai: statusValueFor(to) })
      return true
    },
    [mutations.setValue, statusCol.id],
  )

  const onCardKey = useCallback(
    (e: KeyboardEvent, item: Item, columnId: string) => {
      const action = keyAction(e.key, lifted, item.id, columnIds)
      if (!action) return
      if (action.type !== 'cancel' || e.key !== 'Tab') e.preventDefault()
      switch (action.type) {
        case 'lift':
          if (!item.bisa_ubah) {
            setMessage(`Kartu ${item.judul} hanya dapat dibaca.`)
            return
          }
          setLifted({ itemId: item.id, from: columnId, target: columnId })
          setMessage(`Kartu ${item.judul} diangkat dari kolom ${colName(columnId)}. ${INSTRUCTIONS}`)
          return
        case 'move':
          setLifted((l) => (l ? { ...l, target: action.to } : l))
          setMessage(`Kartu ${item.judul} berada di atas kolom ${colName(action.to)}.`)
          return
        case 'drop': {
          const from = lifted?.from ?? columnId
          setLifted(null)
          if (moveCard(item, from, action.to)) {
            setFocusTarget({ itemId: item.id, columnId: action.to })
            setMessage(`Kartu ${item.judul} dipindah ke kolom ${colName(action.to)}.`)
          } else {
            setMessage(`Kartu ${item.judul} dilepas di kolom yang sama; tidak ada perubahan.`)
          }
          return
        }
        case 'cancel':
          setLifted(null)
          setMessage(`Seret kartu ${item.judul} dibatalkan.`)
      }
    },
    [lifted, columnIds, colName, moveCard],
  )

  const onFocused = useCallback(() => setFocusTarget(null), [])

  const ctx = useMemo<KanbanContextValue>(
    () => ({
      fields: {
        person: firstOf(board.kolom, 'person'),
        priority: firstOf(board.kolom, 'priority'),
        date: firstOf(board.kolom, 'date'),
      } satisfies CardFields,
      names: new Map(members.map((m) => [m.user_id, m.nama])),
      lifted,
      focusTarget,
      onCardKey,
      onFocused,
    }),
    [board.kolom, members, lifted, focusTarget, onCardKey, onFocused],
  )

  const sensors = useSensors(
    useSensor(MouseSensor, { activationConstraint: { distance: MOUSE_DISTANCE } }),
    useSensor(TouchSensor, { activationConstraint: { delay: TOUCH_DELAY_MS, tolerance: TOUCH_TOLERANCE } }),
  )

  const announcements: Announcements = {
    onDragStart: ({ active }) => `Kartu ${title(active.id)} diangkat dari kolom ${colName(active.data.current?.columnId as string)}.`,
    onDragOver: ({ active, over }) =>
      over ? `Kartu ${title(active.id)} berada di atas kolom ${colName(over.id)}.` : `Kartu ${title(active.id)} tidak berada di atas kolom.`,
    onDragEnd: ({ active, over }) =>
      over ? `Kartu ${title(active.id)} dipindah ke kolom ${colName(over.id)}.` : `Kartu ${title(active.id)} dilepas di luar kolom; tidak ada perubahan.`,
    onDragCancel: ({ active }) => `Seret kartu ${title(active.id)} dibatalkan.`,
  }

  function onDragStart(e: DragStartEvent) {
    setLifted(null)
    setActiveId(e.active.id)
  }

  function onDragEnd(e: DragEndEvent) {
    setActiveId(null)
    const item = itemsById.get(String(e.active.id))
    const from = e.active.data.current?.columnId as string | undefined
    if (item && from && e.over) moveCard(item, from, String(e.over.id))
  }

  const activeItem = activeId === null ? undefined : itemsById.get(String(activeId))

  return (
    <KanbanContext.Provider value={ctx}>
      <DndContext
        sensors={sensors}
        collisionDetection={kanbanCollision}
        onDragStart={onDragStart}
        onDragEnd={onDragEnd}
        onDragCancel={() => setActiveId(null)}
        accessibility={{ announcements, screenReaderInstructions: { draggable: INSTRUCTIONS } }}
      >
        <div
          role="region"
          aria-label={`Kanban ${board.nama} menurut ${statusCol.judul}`}
          className="kanban-scroll gap-d flex overflow-x-auto pb-(--space-sm)"
        >
          {columns.map((col) => (
            <KanbanColumn key={col.id} col={col} groups={groups} />
          ))}
        </div>
        <DragOverlay>
          {activeItem ? (
            <div className="w-[calc(var(--kanban-col-w)_-_2*var(--density-pad-x))] cursor-grabbing shadow-lg">
              <CardBody item={activeItem} group={groups.get(activeItem.group_id)} />
            </div>
          ) : null}
        </DragOverlay>
      </DndContext>
      <div role="status" aria-live="assertive" aria-atomic="true" className="sr-only" data-testid="kanban-live">
        {message}
      </div>
    </KanbanContext.Provider>
  )
}
