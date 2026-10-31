import { useEffect, useRef, useState } from 'react'

import type { Column, Item, Member } from '../../api/types'
import { Button } from '../../components/ui'
import {
  asNumber,
  asString,
  asStringArray,
  asTimeline,
  parseNumber,
  parseProgress,
  parseTimeline,
  PRIORITY_STYLE,
  type Parsed,
} from '../format'
import { OptionList, Popover, type Option } from './Popover'

/** Arah perpindahan fokus setelah menyimpan. */
export type Move = 'down' | 'right' | 'left' | null

export interface EditorProps {
  /** null berarti kolom judul item. */
  col: Column | null
  item: Item
  anchor: HTMLElement
  label: string
  /** Karakter yang diketik untuk membuka editor; menggantikan isi awal. */
  initial?: string
  members: Member[]
  onCommit: (value: unknown, move: Move) => void
  onCancel: () => void
  onInvalid: (message: string) => void
}

/** Editor yang tampil di dalam sel (bukan panel mengambang). */
export function isInlineEditor(col: Column | null): boolean {
  return col === null || ['text', 'number', 'progress', 'date'].includes(col.tipe)
}

/** Tipe yang boleh dibuka dengan mengetik langsung. */
export function acceptsTyping(col: Column | null): boolean {
  return col === null || ['text', 'number', 'progress'].includes(col.tipe)
}

export function CellEditor(props: EditorProps) {
  const { col } = props
  if (col === null) return <InlineEditor {...props} />
  switch (col.tipe) {
    case 'text':
    case 'number':
    case 'progress':
    case 'date':
      return <InlineEditor {...props} />
    case 'status':
    case 'priority':
      return <ChoiceEditor {...props} col={col} />
    case 'person':
      return <PersonEditor {...props} col={col} />
    case 'timeline':
      return <TimelineEditor {...props} col={col} />
    case 'dependency':
      return null
  }
}

function initialText(col: Column | null, item: Item): string {
  if (col === null) return item.judul
  const v = item.nilai[col.id]
  if (col.tipe === 'number' || col.tipe === 'progress') {
    const n = asNumber(v)
    return n === null ? '' : String(n).replace('.', ',')
  }
  return asString(v) ?? ''
}

function parseInline(col: Column | null, text: string): Parsed<unknown> {
  if (col === null) {
    const judul = text.trim()
    return judul ? { ok: true, value: judul } : { ok: false, message: 'Judul item wajib diisi.' }
  }
  switch (col.tipe) {
    case 'number':
      return parseNumber(text)
    case 'progress':
      return parseProgress(text)
    case 'text':
      return { ok: true, value: text.trim() === '' ? null : text }
    default:
      return { ok: true, value: text === '' ? null : text }
  }
}

function InlineEditor({ col, item, label, initial, onCommit, onCancel, onInvalid }: EditorProps) {
  const [text, setText] = useState(() => initial ?? initialText(col, item))
  const [invalid, setInvalid] = useState(false)
  const ref = useRef<HTMLInputElement>(null)
  const done = useRef(false)

  useEffect(() => {
    const el = ref.current
    if (!el) return
    el.focus()
    if (initial === undefined && el.type !== 'date') el.select()
  }, [initial])

  function commit(move: Move): boolean {
    const p = parseInline(col, text)
    if (!p.ok) {
      setInvalid(true)
      onInvalid(p.message)
      return false
    }
    done.current = true
    onCommit(p.value, move)
    return true
  }

  const isDate = col?.tipe === 'date'
  const numeric = col?.tipe === 'number' || col?.tipe === 'progress'
  return (
    <input
      ref={ref}
      aria-label={label}
      aria-invalid={invalid || undefined}
      type={isDate ? 'date' : 'text'}
      inputMode={numeric ? 'decimal' : undefined}
      maxLength={col === null ? 500 : col.tipe === 'text' ? 2000 : undefined}
      value={text}
      onChange={(e) => {
        setInvalid(false)
        setText(e.target.value)
      }}
      onKeyDown={(e) => {
        if (e.key === 'Enter') {
          e.preventDefault()
          commit('down')
        } else if (e.key === 'Tab') {
          e.preventDefault()
          commit(e.shiftKey ? 'left' : 'right')
        } else if (e.key === 'Escape') {
          e.preventDefault()
          done.current = true
          onCancel()
        }
        e.stopPropagation()
      }}
      onBlur={() => {
        // Klik di luar menyimpan; nilai tidak sah dibatalkan agar sel tidak terkunci.
        if (done.current) return
        if (!commit(null)) {
          done.current = true
          onCancel()
        }
      }}
      className={`board-cell px-d w-full border-(length:--focus-ring-w) bg-surface outline-none ${invalid ? 'border-danger' : 'border-primary'} ${
        numeric ? 'text-right tabular-nums' : ''
      }`}
    />
  )
}

function ChoiceEditor({ col, item, anchor, label, onCommit, onCancel }: EditorProps & { col: Column }) {
  const current = asString(item.nilai[col.id]) ?? ''
  const options: Option[] =
    col.tipe === 'priority'
      ? Object.entries(PRIORITY_STYLE).map(([value, s]) => ({ value, label: s.judul, warna: s.warna }))
      : (col.konfigurasi.labels ?? []).map((l) => ({ value: l.id, label: l.judul, warna: l.warna }))
  options.push({ value: '', label: 'Kosongkan' })

  return (
    <Popover anchor={anchor} label={label} onClose={onCancel}>
      <OptionList
        label={label}
        options={options}
        selected={current}
        onSelect={(v) => onCommit(v === '' ? null : v, null)}
        onCancel={onCancel}
      />
    </Popover>
  )
}

function PersonEditor({ col, item, anchor, label, members, onCommit, onCancel }: EditorProps & { col: Column }) {
  const [chosen, setChosen] = useState<string[]>(() => asStringArray(item.nilai[col.id]))
  const firstRef = useRef<HTMLInputElement>(null)
  // Hanya anggota yang masih punya akses; server menolak orang di luar proyek.
  const candidates = members.filter((m) => m.aktif && m.peran_efektif !== '')

  useEffect(() => {
    firstRef.current?.focus()
  }, [])

  const toggle = (id: string) => setChosen((c) => (c.includes(id) ? c.filter((x) => x !== id) : [...c, id]))
  const save = () => onCommit(chosen.length ? chosen : null, null)

  return (
    <Popover anchor={anchor} label={label} onClose={onCancel}>
      <div
        className="py-d flex flex-col"
        onKeyDown={(e) => {
          if (e.key === 'Escape') {
            e.preventDefault()
            onCancel()
          } else if (e.key === 'Enter' && (e.target as HTMLElement).tagName === 'INPUT') {
            e.preventDefault()
            save()
          }
          e.stopPropagation()
        }}
      >
        <fieldset className="max-h-(--panel-max-h) overflow-auto">
          <legend className="sr-only">{label}</legend>
          {candidates.length === 0 && <p className="px-d text-muted-foreground">Belum ada anggota proyek.</p>}
          {candidates.map((m, i) => (
            <label key={m.user_id} className="px-d row-h flex cursor-pointer items-center gap-d hover:bg-canvas">
              <input
                ref={i === 0 ? firstRef : undefined}
                type="checkbox"
                checked={chosen.includes(m.user_id)}
                onChange={() => toggle(m.user_id)}
              />
              <span className="truncate">{m.nama}</span>
            </label>
          ))}
        </fieldset>
        <div className="px-d gap-d flex justify-end border-t border-line pt-(--space-sm)">
          <Button onClick={onCancel}>Batal</Button>
          <Button variant="primary" onClick={save}>
            Simpan
          </Button>
        </div>
      </div>
    </Popover>
  )
}

function TimelineEditor({ col, item, anchor, label, onCommit, onCancel, onInvalid }: EditorProps & { col: Column }) {
  const t = asTimeline(item.nilai[col.id])
  const [mulai, setMulai] = useState(t?.mulai ?? '')
  const [selesai, setSelesai] = useState(t?.selesai ?? '')
  const firstRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    firstRef.current?.focus()
  }, [])

  const save = () => {
    const p = parseTimeline(mulai, selesai)
    if (p.ok) onCommit(p.value, null)
    else onInvalid(p.message)
  }

  return (
    <Popover anchor={anchor} label={label} onClose={onCancel}>
      <div
        className="px-d py-d gap-d flex flex-col"
        onKeyDown={(e) => {
          if (e.key === 'Escape') {
            e.preventDefault()
            onCancel()
          } else if (e.key === 'Enter' && (e.target as HTMLElement).tagName === 'INPUT') {
            e.preventDefault()
            save()
          }
          e.stopPropagation()
        }}
      >
        <label className="flex flex-col gap-(--space-sm)">
          Mulai
          <input ref={firstRef} type="date" className="row-h px-d rounded border border-line" value={mulai} onChange={(e) => setMulai(e.target.value)} />
        </label>
        <label className="flex flex-col gap-(--space-sm)">
          Selesai
          <input type="date" className="row-h px-d rounded border border-line" value={selesai} onChange={(e) => setSelesai(e.target.value)} />
        </label>
        <div className="gap-d flex justify-end">
          <Button onClick={() => onCommit(null, null)}>Kosongkan</Button>
          <Button variant="primary" onClick={save}>
            Simpan
          </Button>
        </div>
      </div>
    </Popover>
  )
}
