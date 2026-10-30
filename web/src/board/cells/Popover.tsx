import { useEffect, useId, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'

/**
 * Panel mengambang di bawah elemen jangkar, dirender ke body agar tidak
 * terpotong area gulir tabel. Menutup saat klik di luar atau saat tabel
 * digulir (posisi jangkar tidak lagi valid).
 */
export function Popover({
  anchor,
  label,
  onClose,
  children,
}: {
  anchor: HTMLElement
  label: string
  onClose: () => void
  children: ReactNode
}) {
  const ref = useRef<HTMLDivElement>(null)
  const [pos, setPos] = useState<{ top: number; left: number } | null>(null)

  useLayoutEffect(() => {
    const r = anchor.getBoundingClientRect()
    const panel = ref.current
    const w = panel?.offsetWidth ?? 0
    const h = panel?.offsetHeight ?? 0
    const left = Math.max(4, Math.min(r.left, window.innerWidth - w - 4))
    const below = r.bottom + 2
    const top = below + h > window.innerHeight && r.top - h - 2 > 0 ? r.top - h - 2 : below
    setPos({ top, left })
  }, [anchor])

  useEffect(() => {
    // Tutup hanya sekali: menutup bisa memicu gulir (fokus kembali ke sel) yang
    // memanggil onScroll lagi sebelum panel dilepas.
    let closed = false
    const close = () => {
      if (closed) return
      closed = true
      onClose()
    }
    const onDown = (e: MouseEvent) => {
      const t = e.target as Node
      if (!ref.current?.contains(t) && !anchor.contains(t)) close()
    }
    const onScroll = (e: Event) => {
      if (!ref.current?.contains(e.target as Node)) close()
    }
    document.addEventListener('mousedown', onDown)
    window.addEventListener('scroll', onScroll, true)
    return () => {
      document.removeEventListener('mousedown', onDown)
      window.removeEventListener('scroll', onScroll, true)
    }
  }, [anchor, onClose])

  return createPortal(
    <div
      ref={ref}
      role="dialog"
      aria-label={label}
      className="fixed z-50 min-w-(--panel-min-w) rounded border border-line bg-surface shadow-lg"
      style={{ top: pos?.top ?? -9999, left: pos?.left ?? -9999 }}
    >
      {children}
    </div>,
    document.body,
  )
}

export interface Option {
  value: string
  label: string
  warna?: string
}

/**
 * Daftar pilihan tunggal dengan keyboard: panah atas/bawah, Home/End, Enter
 * memilih, Esc membatalkan. Nilai '' berarti kosongkan.
 */
export function OptionList({
  label,
  options,
  selected,
  onSelect,
  onCancel,
}: {
  label: string
  options: Option[]
  selected: string
  onSelect: (value: string) => void
  onCancel: () => void
}) {
  // Mulai dari pilihan saat ini; bila sel kosong, dari pilihan pertama (bukan "Kosongkan").
  const [index, setIndex] = useState(() => (selected ? Math.max(0, options.findIndex((o) => o.value === selected)) : 0))
  const listRef = useRef<HTMLUListElement>(null)
  const base = useId()

  useEffect(() => {
    listRef.current?.focus()
  }, [])

  return (
    <ul
      ref={listRef}
      role="listbox"
      aria-label={label}
      tabIndex={0}
      aria-activedescendant={`${base}-${index}`}
      className="max-h-(--panel-max-h) overflow-auto py-(--space-sm) outline-none"
      onKeyDown={(e) => {
        const last = options.length - 1
        const keys: Record<string, () => void> = {
          ArrowDown: () => setIndex((i) => Math.min(last, i + 1)),
          ArrowUp: () => setIndex((i) => Math.max(0, i - 1)),
          Home: () => setIndex(0),
          End: () => setIndex(last),
          Enter: () => {
            const o = options[index]
            if (o) onSelect(o.value)
          },
          ' ': () => {
            const o = options[index]
            if (o) onSelect(o.value)
          },
          Escape: onCancel,
          Tab: onCancel,
        }
        const fn = keys[e.key]
        if (fn) {
          e.preventDefault()
          e.stopPropagation()
          fn()
        }
      }}
    >
      {options.map((o, i) => (
        <li
          key={o.value}
          id={`${base}-${i}`}
          role="option"
          aria-selected={o.value === selected}
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => onSelect(o.value)}
          onMouseEnter={() => setIndex(i)}
          className={`px-d row-h flex cursor-pointer items-center gap-d ${i === index ? 'bg-canvas' : ''}`}
        >
          {o.warna ? (
            <span className={`tag-${o.warna} tag-fill inline-flex min-w-(--badge-min-w) justify-center rounded px-d`}>{o.label}</span>
          ) : (
            <span className="text-muted-foreground">{o.label}</span>
          )}
          {o.value === selected && <span aria-hidden="true">✓</span>}
        </li>
      ))}
    </ul>
  )
}
