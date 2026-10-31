import type { Column, Member } from '../../api/types'
import { PRIORITY_STYLE } from '../format'

const FIELD = 'row-h px-d rounded border border-line bg-surface'

/**
 * Isian nilai kolom untuk kondisi dan aksi otomasi, sesuai tipe kolom.
 * Nilai kosong (null) berarti "kosongkan" / "kosong".
 */
export function ValueInput({
  col,
  value,
  members,
  label,
  onChange,
}: {
  col: Column
  value: unknown
  members: Member[]
  label: string
  onChange: (v: unknown) => void
}) {
  switch (col.tipe) {
    case 'status':
    case 'priority': {
      const options =
        col.tipe === 'status'
          ? (col.konfigurasi.labels ?? []).map((l) => ({ value: l.id, label: l.judul }))
          : Object.entries(PRIORITY_STYLE).map(([value, s]) => ({ value, label: s.judul }))
      return (
        <select aria-label={label} className={FIELD} value={typeof value === 'string' ? value : ''} onChange={(e) => onChange(e.target.value || null)}>
          <option value="">Kosong</option>
          {options.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      )
    }
    case 'person': {
      const chosen = Array.isArray(value) ? value.map(String) : []
      return (
        <select
          aria-label={label}
          className={FIELD}
          value={chosen[0] ?? ''}
          onChange={(e) => onChange(e.target.value ? [e.target.value] : null)}
        >
          <option value="">Kosong</option>
          {members
            .filter((m) => m.aktif && m.peran_efektif !== '')
            .map((m) => (
              <option key={m.user_id} value={m.user_id}>
                {m.nama}
              </option>
            ))}
        </select>
      )
    }
    case 'date':
      return (
        <input
          aria-label={label}
          type="date"
          className={FIELD}
          value={typeof value === 'string' ? value : ''}
          onChange={(e) => onChange(e.target.value || null)}
        />
      )
    case 'number':
    case 'progress':
      return (
        <input
          aria-label={label}
          type="number"
          min={col.tipe === 'progress' ? 0 : undefined}
          max={col.tipe === 'progress' ? 100 : undefined}
          className={`${FIELD} tabular-nums`}
          value={typeof value === 'number' ? value : ''}
          onChange={(e) => onChange(e.target.value === '' ? null : Number(e.target.value))}
        />
      )
    default:
      return (
        <input
          aria-label={label}
          className={FIELD}
          maxLength={2000}
          value={typeof value === 'string' ? value : ''}
          onChange={(e) => onChange(e.target.value || null)}
        />
      )
  }
}
