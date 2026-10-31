import type { Column, Item } from '../../api/types'
import { DependencyCellText } from '../deps/DependencyContext'
import {
  asNumber,
  asString,
  asStringArray,
  asTimeline,
  formatDate,
  formatNumber,
  formatTimeline,
  initials,
  isPriority,
  PRIORITY_STYLE,
} from '../format'

/** Teks ringkas nilai sel untuk label aksesibilitas dan tooltip. */
export function valueText(col: Column, item: Item, names: ReadonlyMap<string, string>): string {
  const v = item.nilai[col.id]
  switch (col.tipe) {
    case 'text':
      return asString(v) ?? ''
    case 'status': {
      const id = asString(v)
      return col.konfigurasi.labels?.find((l) => l.id === id)?.judul ?? ''
    }
    case 'priority':
      return isPriority(v) ? PRIORITY_STYLE[v].judul : ''
    case 'person':
      return asStringArray(v)
        .map((id) => names.get(id) ?? 'Pengguna')
        .join(', ')
    case 'date':
      return formatDate(asString(v))
    case 'timeline':
      return formatTimeline(asTimeline(v))
    case 'number':
      return formatNumber(asNumber(v), col.konfigurasi.satuan)
    case 'progress': {
      const n = asNumber(v)
      return n === null ? '' : `${n}%`
    }
    case 'dependency':
      return ''
  }
}

function Badge({ warna, children }: { warna: string; children: string }) {
  return (
    <span className={`tag-${warna} tag-fill flex h-full w-full items-center justify-center truncate px-(--space-sm)`}>
      {children}
    </span>
  )
}

/** Tampilan nilai sel (bukan mode ubah). */
export function CellDisplay({ col, item, names }: { col: Column; item: Item; names: ReadonlyMap<string, string> }) {
  const v = item.nilai[col.id]
  switch (col.tipe) {
    case 'status': {
      const id = asString(v)
      const label = col.konfigurasi.labels?.find((l) => l.id === id)
      return label ? <Badge warna={label.warna}>{label.judul}</Badge> : null
    }
    case 'priority':
      return isPriority(v) ? <Badge warna={PRIORITY_STYLE[v].warna}>{PRIORITY_STYLE[v].judul}</Badge> : null
    case 'person': {
      const ids = asStringArray(v)
      if (ids.length === 0) return null
      const list = ids.map((id) => names.get(id) ?? 'Pengguna')
      return (
        <span className="px-d flex h-full items-center gap-(--space-sm)" title={list.join(', ')}>
          {list.slice(0, 3).map((nama, i) => (
            <span
              key={ids[i]}
              aria-hidden="true"
              className="inline-flex size-(--avatar-size) shrink-0 items-center justify-center rounded-full bg-primary text-(length:--font-xs) font-semibold text-white"
            >
              {initials(nama)}
            </span>
          ))}
          {list.length === 1 ? (
            <span className="truncate">{list[0]}</span>
          ) : list.length > 3 ? (
            <span className="text-muted-foreground">+{list.length - 3}</span>
          ) : null}
        </span>
      )
    }
    case 'progress': {
      const n = asNumber(v)
      if (n === null) return null
      return (
        <span className="px-d flex h-full items-center gap-d">
          <span className="h-(--bar-h) flex-1 overflow-hidden rounded-full bg-canvas">
            <span className="block h-full rounded-full bg-primary" style={{ width: `${Math.min(100, Math.max(0, n))}%` }} />
          </span>
          <span className="w-(--pct-w) text-right tabular-nums">{n}%</span>
        </span>
      )
    }
    case 'number':
      return (
        <span className="px-d flex h-full items-center justify-end tabular-nums">
          {formatNumber(asNumber(v), col.konfigurasi.satuan)}
        </span>
      )
    case 'dependency':
      return <DependencyCellText item={item} />
    default:
      return <span className="px-d flex h-full items-center truncate">{valueText(col, item, names)}</span>
  }
}
