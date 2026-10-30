import type { ComponentProps } from 'react'

import type { GanttChart as GanttChartType } from './gantt/GanttChart'
import { LazyView, type ViewLoader } from './LazyView'

type GanttProps = ComponentProps<typeof GanttChartType>
export type GanttLoader = ViewLoader<GanttProps>

const loadGantt: GanttLoader = () => import('./gantt/GanttChart').then((m) => ({ default: m.GanttChart }))

/** Gantt yang dimuat saat tab Gantt dibuka. */
export function LazyGantt({ load = loadGantt, ...props }: GanttProps & { load?: GanttLoader }) {
  return <LazyView name="Gantt" load={load} props={props} />
}
