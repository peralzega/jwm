import { useCallback, useEffect, useId, useMemo, useRef, useState, type KeyboardEvent, type ReactNode } from 'react'

import { PopoverAnchor, PopoverContent, Popover as UiPopover } from '@/components/ui/popover'

const FOCUSABLE = 'button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea, [tabindex]:not([tabindex="-1"])'

/**
 * Panel editor sel (shadcn Popover, non-modal) yang menempel pada sel tabel.
 * - Esc dan klik di luar menutup; fokus dikembalikan ke sel oleh BoardTable
 *   (onCloseAutoFocus dicegah agar Radix tidak memindah fokus ke body).
 * - Tab berputar di dalam panel, tidak lepas ke luar tabel.
 * - Menggulir di luar panel menutup panel: sel jangkar bisa dilepas oleh
 *   virtualisasi sehingga posisinya tidak lagi valid.
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
  const contentRef = useRef<HTMLDivElement>(null)
  const virtualRef = useMemo(() => ({ current: anchor }), [anchor])
  // Tutup hanya sekali: menutup bisa memicu gulir (fokus kembali ke sel) dan
  // Esc ditangani baik oleh Radix maupun oleh isi panel.
  const closed = useRef(false)
  const close = useCallback(() => {
    if (closed.current) return
    closed.current = true
    onClose()
  }, [onClose])

  useEffect(() => {
    const onScroll = (e: Event) => {
      if (!contentRef.current?.contains(e.target as Node)) close()
    }
    window.addEventListener('scroll', onScroll, true)
    return () => window.removeEventListener('scroll', onScroll, true)
  }, [close])

  function trapTab(e: KeyboardEvent<HTMLDivElement>) {
    if (e.key !== 'Tab') return
    const items = [...(contentRef.current?.querySelectorAll<HTMLElement>(FOCUSABLE) ?? [])]
    const first = items[0]
    const last = items[items.length - 1]
    if (!first || !last) {
      e.preventDefault()
      return
    }
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault()
      first.focus()
    }
  }

  return (
    <UiPopover
      open
      onOpenChange={(open) => {
        if (!open) close()
      }}
    >
      <PopoverAnchor virtualRef={virtualRef} />
      <PopoverContent
        ref={contentRef}
        aria-label={label}
        // Radix memfokuskan elemen pertama yang dapat difokus (daftar pilihan,
        // kotak centang pertama, atau tanggal mulai) setelah Portal terpasang.
        onCloseAutoFocus={(e) => e.preventDefault()}
        // Esc ditangani editor di dalam panel (lewat event React) agar penutupan dan
        // pengembalian fokus ke sel terjadi sekali, dalam urutan yang sama dengan tabel.
        onEscapeKeyDown={(e) => {
          if (contentRef.current?.contains(document.activeElement)) e.preventDefault()
        }}
        onInteractOutside={(e) => {
          // Klik pada sel asal tidak menutup panel (sel itu sendiri yang membukanya).
          if (anchor.contains(e.target as Node)) e.preventDefault()
        }}
        // Fase capture: editor di dalam panel menghentikan propagasi keydown.
        onKeyDownCapture={trapTab}
      >
        {children}
      </PopoverContent>
    </UiPopover>
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
