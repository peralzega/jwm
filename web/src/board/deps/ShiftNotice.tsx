import type { ShiftedItem } from '../../api/types'
import { Alert, Button } from '../../components/ui'
import { formatTimeline } from '../format'

const MAX_LISTED = 5

/** Pemberitahuan successor yang ikut digeser otomatis oleh perubahan timeline. */
export function ShiftNotice({ items, onDismiss }: { items: ShiftedItem[]; onDismiss: () => void }) {
  const listed = items.slice(0, MAX_LISTED)
  const rest = items.length - listed.length
  return (
    <Alert kind="ok">
      <span className="gap-d flex flex-wrap items-center justify-between">
        <span>
          {items.length} item ikut bergeser agar urutan dependensi terpenuhi:{' '}
          {listed.map((s) => `${s.judul} (${formatTimeline({ mulai: s.mulai, selesai: s.selesai })})`).join(', ')}
          {rest > 0 && `, dan ${rest} item lain`}.
        </span>
        <Button onClick={onDismiss}>Tutup</Button>
      </span>
    </Alert>
  )
}
