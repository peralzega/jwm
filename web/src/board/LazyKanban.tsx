import type { ComponentProps } from 'react'

import type { KanbanBoard as KanbanBoardType } from './kanban/KanbanBoard'
import { LazyView, type ViewLoader } from './LazyView'

type KanbanProps = ComponentProps<typeof KanbanBoardType>
export type KanbanLoader = ViewLoader<KanbanProps>

const loadKanban: KanbanLoader = () => import('./kanban/KanbanBoard').then((m) => ({ default: m.KanbanBoard }))

/** Kanban (beserta dnd-kit) yang dimuat saat tab Kanban dibuka. */
export function LazyKanban({ load = loadKanban, ...props }: KanbanProps & { load?: KanbanLoader }) {
  return <LazyView name="Kanban" load={load} props={props} />
}
