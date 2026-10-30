import { closestCenter, pointerWithin, type CollisionDetection } from '@dnd-kit/core'

/** Mouse/sentuh: kolom di bawah penunjuk; bila penunjuk di sela kolom, kolom terdekat. */
export const kanbanCollision: CollisionDetection = (args) => {
  const hits = args.pointerCoordinates ? pointerWithin(args) : []
  return hits.length > 0 ? hits : closestCenter(args)
}

/** Kartu yang sedang diangkat dengan keyboard dan kolom tujuannya. */
export interface Lifted {
  itemId: string
  from: string
  target: string
}

export type KeyAction =
  | { type: 'lift' }
  | { type: 'move'; to: string }
  | { type: 'drop'; to: string }
  | { type: 'cancel' }
  | null

/**
 * Seret dengan keyboard tanpa geometri: Spasi/Enter mengangkat dan melepas,
 * panah kiri/kanan memilih kolom tujuan, Esc (atau Tab) membatalkan.
 * Sensor keyboard dnd-kit tidak dipakai karena menggulir area Kanban alih-alih
 * memindah kartu saat kolom tujuan ada di separuh kiri area gulir, lalu gagal
 * mendeteksi kolom baru ketika kartu diseret lewat DragOverlay.
 */
export function keyAction(key: string, lifted: Lifted | null, itemId: string, columnIds: readonly string[]): KeyAction {
  const mine = lifted?.itemId === itemId ? lifted : null
  if (!mine) return key === ' ' || key === 'Enter' ? { type: 'lift' } : null
  const i = columnIds.indexOf(mine.target)
  switch (key) {
    case 'ArrowLeft':
    case 'ArrowRight': {
      const to = columnIds[key === 'ArrowLeft' ? i - 1 : i + 1]
      return to === undefined ? null : { type: 'move', to }
    }
    case ' ':
    case 'Enter':
      return { type: 'drop', to: mine.target }
    case 'Escape':
    case 'Tab':
      return { type: 'cancel' }
    default:
      return null
  }
}
