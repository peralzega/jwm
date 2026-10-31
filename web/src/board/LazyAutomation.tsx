import type { ComponentProps } from 'react'

import type { AutomationPanel as AutomationPanelType } from './automation/AutomationPanel'
import { LazyView, type ViewLoader } from './LazyView'

type AutomationProps = ComponentProps<typeof AutomationPanelType>
export type AutomationLoader = ViewLoader<AutomationProps>

const loadAutomation: AutomationLoader = () =>
  import('./automation/AutomationPanel').then((m) => ({ default: m.AutomationPanel }))

/** Tab Otomasi yang dimuat saat dibuka. */
export function LazyAutomation({ load = loadAutomation, ...props }: AutomationProps & { load?: AutomationLoader }) {
  return <LazyView name="Otomasi" load={load} props={props} />
}
