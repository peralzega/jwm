import { useRef, type KeyboardEvent } from 'react'

export type BoardView = 'tabel' | 'kanban'

const VIEWS: { id: BoardView; label: string }[] = [
  { id: 'tabel', label: 'Tabel' },
  { id: 'kanban', label: 'Kanban' },
]

export function parseView(v: string | null): BoardView {
  return v === 'kanban' ? 'kanban' : 'tabel'
}

/** Tab tampilan board (pola tablist: panah kiri/kanan berpindah tab). */
export function ViewTabs({ value, onChange }: { value: BoardView; onChange: (v: BoardView) => void }) {
  const refs = useRef<(HTMLButtonElement | null)[]>([])

  function onKeyDown(e: KeyboardEvent, i: number) {
    if (e.key !== 'ArrowRight' && e.key !== 'ArrowLeft') return
    e.preventDefault()
    const next = (i + (e.key === 'ArrowRight' ? 1 : VIEWS.length - 1)) % VIEWS.length
    const v = VIEWS[next]
    if (!v) return
    onChange(v.id)
    refs.current[next]?.focus()
  }

  return (
    <div role="tablist" aria-label="Tampilan board" className="flex border-b border-line">
      {VIEWS.map((v, i) => {
        const selected = v.id === value
        return (
          <button
            key={v.id}
            ref={(el) => {
              refs.current[i] = el
            }}
            type="button"
            role="tab"
            aria-selected={selected}
            tabIndex={selected ? 0 : -1}
            onClick={() => onChange(v.id)}
            onKeyDown={(e) => onKeyDown(e, i)}
            className={`px-d row-h -mb-(--line-w) border-b-(length:--tab-line-w) outline-none focus-visible:ring-(length:--focus-ring-w) focus-visible:ring-accent ${
              selected ? 'border-accent font-semibold' : 'border-transparent text-muted hover:text-ink'
            }`}
          >
            {v.label}
          </button>
        )
      })}
    </div>
  )
}
