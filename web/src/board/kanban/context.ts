import { createContext, type KeyboardEvent } from 'react'

import type { Item } from '../../api/types'
import type { CardFields } from './KanbanCard'
import type { Lifted } from './keyboard'

export interface KanbanContextValue {
  fields: CardFields
  names: ReadonlyMap<string, string>
  /** Kartu yang sedang diangkat dengan keyboard. */
  lifted: Lifted | null
  /** Kartu yang perlu menerima fokus setelah dilepas: id item dan kolom tujuannya. */
  focusTarget: { itemId: string; columnId: string } | null
  onCardKey: (e: KeyboardEvent, item: Item, columnId: string) => void
  onFocused: () => void
}

export const KanbanContext = createContext<KanbanContextValue>({
  fields: {},
  names: new Map(),
  lifted: null,
  focusTarget: null,
  onCardKey: () => undefined,
  onFocused: () => undefined,
})
