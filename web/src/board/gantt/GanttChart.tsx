import { useVirtualizer } from '@tanstack/react-virtual'
import {
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent,
  type PointerEvent,
} from 'react'

import { errorMessage } from '../../api/client'
import type { BoardDetail, Column, Item, ItemSchedule } from '../../api/types'
import { Alert } from '../../components/ui'
import { cssPx, useDensity } from '../../density'
import { CellEditor } from '../cells/editors'
import { useDepsContext } from '../deps/DependencyContext'
import { DependencyEditor } from '../deps/DependencyEditor'
import { useSchedule } from '../deps/queries'
import { asTimeline, formatDate, isValidDate } from '../format'
import { buildRows, countItems, type GridRow } from '../rows'
import type { ItemMutations } from '../useBoardItems'
import { buildArrows } from './arrows'
import {
  applyDrag,
  barBox,
  chartRange,
  dayNum,
  daysFromPx,
  describeSpan,
  headerTicks,
  isoOf,
  todayNum,
  xOf,
  type DragMode,
  type GanttScale,
  type Span,
} from './scale'

// Gantt board: panel judul menempel di kiri, header dua tingkat menempel di
// atas, baris tinggi tetap yang divirtualisasi (sama dengan tabel). Batang
// digeser dengan seret atau keyboard; perubahan disimpan ke kolom timeline.
// Jalur kritis, pelanggaran FS (dengan lag dan hari kerja), dan pergeseran
// otomatis successor dihitung backend (GET /schedule, PUT nilai timeline).

const DAY_FALLBACK: Record<GanttScale, number> = { hari: 28, minggu: 12, bulan: 4 }
// Jeda sebelum perubahan dari keyboard disimpan, agar tekan beruntun jadi satu permintaan.
const KEY_COMMIT_MS = 600
const NO_SCHEDULE: ReadonlyMap<string, ItemSchedule> = new Map()

// committing: draf sudah dikirim ke server dan menunggu jawaban; jangan dikirim ulang.
type Draft = { id: string; span: Span; committing?: boolean }
type Drag = { id: string; mode: DragMode; startX: number; base: Span; delta: number }

function readSpan(v: unknown): Span | null {
  const t = asTimeline(v)
  if (!t || !isValidDate(t.mulai) || !isValidDate(t.selesai)) return null
  const start = dayNum(t.mulai)
  const end = dayNum(t.selesai)
  return end >= start ? { start, end } : null
}

const sameSpan = (a: Span | undefined, b: Span) => !!a && a.start === b.start && a.end === b.end

export function GanttChart({
  board,
  items,
  timelineCol,
  scale,
  mutations,
  onError,
  today = todayNum(),
}: {
  board: BoardDetail
  items: Item[]
  timelineCol: Column
  scale: GanttScale
  mutations: ItemMutations
  onError: (message: string | null) => void
  /** Hari ini (nomor hari UTC); dapat diisi tes. */
  today?: number
}) {
  const depsCtx = useDepsContext()
  const schedule = useSchedule(board.id, timelineCol.id)
  const schedById = useMemo(
    () => (schedule.data ? new Map(schedule.data.items.map((s) => [s.item_id, s])) : NO_SCHEDULE),
    [schedule.data],
  )
  // Pergeseran otomatis hanya berlaku pada kolom timeline pertama (sama dengan backend).
  const primaryCol = useMemo(
    () => [...board.kolom].filter((c) => c.tipe === 'timeline').sort((a, b) => a.urutan - b.urutan)[0],
    [board.kolom],
  )

  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(() => new Set())
  const [activeRow, setActiveRow] = useState(-1)
  const [draft, setDraft] = useState<Draft | null>(null)
  const [editing, setEditing] = useState<{ id: string; anchor: HTMLElement } | null>(null)
  const [depEditing, setDepEditing] = useState<{ id: string; anchor: HTMLElement } | null>(null)
  const [announce, setAnnounce] = useState('')
  const scrollRef = useRef<HTMLDivElement>(null)
  const focusPending = useRef(false)
  const dragRef = useRef<Drag | null>(null)
  const keyTimer = useRef<number | undefined>(undefined)
  const helpId = useId()

  const rows = useMemo(() => buildRows(board.grup, items, collapsed, false), [board.grup, items, collapsed])
  const byId = useMemo(() => new Map(items.map((it) => [it.id, it])), [items])
  const baseSpans = useMemo(() => {
    const m = new Map<string, Span>()
    for (const it of items) {
      const s = readSpan(it.nilai[timelineCol.id])
      if (s) m.set(it.id, s)
    }
    return m
  }, [items, timelineCol.id])
  const spans = useMemo(() => {
    if (!draft) return baseSpans
    const m = new Map(baseSpans)
    m.set(draft.id, draft.span)
    return m
  }, [baseSpans, draft])
  const rowIndex = useMemo(() => {
    const m = new Map<string, number>()
    rows.forEach((r, i) => {
      if (r.kind === 'item') m.set(r.item.id, i)
    })
    return m
  }, [rows])

  // Rentang dari jadwal tersimpan (bukan draf) agar sumbu tidak melompat saat menyeret.
  const range = useMemo(() => chartRange([...baseSpans.values()], today, scale), [baseSpans, today, scale])

  const density = useDensity()
  /* eslint-disable react-hooks/exhaustive-deps -- token dibaca ulang saat kepadatan berganti */
  const rowPx = useMemo(() => cssPx('--density-row', 32), [density])
  const dayW = useMemo(() => cssPx(`--gantt-day-${scale}`, DAY_FALLBACK[scale]), [density, scale])
  const gap = useMemo(() => cssPx('--gantt-arrow-gap', 8), [density])
  const listW = useMemo(() => cssPx('--gantt-list-w', 280), [density])
  /* eslint-enable react-hooks/exhaustive-deps */
  const totalW = (range.end - range.start) * dayW
  const headerH = 2 * rowPx

  const ticks = useMemo(() => headerTicks(range, scale, dayW), [range, scale, dayW])
  // Jadwal tersimpan saat hasil CPM terakhir tiba. Item yang jadwalnya berbeda
  // sejak itu (atau sedang diseret) memakai pemeriksaan FS lokal sampai CPM
  // dihitung ulang, agar panah tidak menampilkan status lama.
  // eslint-disable-next-line react-hooks/exhaustive-deps -- sengaja hanya diperbarui saat data jadwal berganti
  const scheduledSpans = useMemo(() => baseSpans, [schedule.data])
  const arrows = useMemo(() => {
    let server: { violated: ReadonlySet<string>; local: ReadonlySet<string> } | undefined
    if (schedule.data) {
      const local = new Set<string>()
      for (const [id, s] of spans) {
        if (!sameSpan(scheduledSpans.get(id), s)) local.add(id)
      }
      for (const id of scheduledSpans.keys()) if (!spans.has(id)) local.add(id)
      server = { violated: new Set(schedule.data.dependensi_dilanggar), local }
    }
    return buildArrows(depsCtx.deps, spans, rowIndex, { range, dayW, rowH: rowPx, gap }, server)
  }, [depsCtx.deps, spans, rowIndex, range, dayW, rowPx, gap, schedule.data, scheduledSpans])

  // Proyek ini tidak memakai React Compiler; peringatan memoisasi tidak berlaku.
  // eslint-disable-next-line react-hooks/incompatible-library
  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => rowPx,
    getItemKey: (i) => rows[i]?.key ?? i,
    overscan: 6,
    scrollMargin: headerH,
    scrollPaddingStart: headerH,
  })

  useEffect(() => {
    virtualizer.measure()
  }, [rowPx, virtualizer])

  // Saat dibuka atau skala/kepadatan berganti (bukan tiap perubahan data):
  // hari ini tampil dekat tepi kiri area waktu.
  const scrolledFor = useRef('')
  useLayoutEffect(() => {
    const sc = scrollRef.current
    const key = `${scale}:${dayW}`
    if (!sc || scrolledFor.current === key) return
    scrolledFor.current = key
    sc.scrollLeft = Math.max(0, xOf(today, range, dayW) - 7 * dayW)
  }, [today, range, dayW, scale])

  const firstFocusable = rows.findIndex((r) => r.kind === 'item' || r.kind === 'group')
  const current = activeRow >= 0 && activeRow < rows.length ? activeRow : firstFocusable

  const focusRow = useCallback((i: number, tries = 6) => {
    const el = scrollRef.current?.querySelector<HTMLElement>(`[data-gantt-row="${i}"]`)
    if (!el) {
      if (tries > 0) requestAnimationFrame(() => focusRow(i, tries - 1))
      return
    }
    el.focus({ preventScroll: true })
    // Batang di luar area horizontal: gulir agar terlihat di samping panel judul.
    const sc = scrollRef.current
    if (sc && el.dataset.left) {
      const left = Number(el.dataset.left)
      const width = Number(el.dataset.width)
      const viewLeft = sc.scrollLeft
      const viewRight = sc.scrollLeft + sc.clientWidth - listW
      if (left < viewLeft || left + width > viewRight) sc.scrollLeft = Math.max(0, left - 2 * dayW)
    }
  }, [listW, dayW])

  const moveTo = useCallback(
    (i: number) => {
      const r = Math.max(0, Math.min(rows.length - 1, i))
      focusPending.current = true
      setActiveRow(r)
      virtualizer.scrollToIndex(r, { align: 'auto' })
    },
    [rows.length, virtualizer],
  )

  useEffect(() => {
    if (!focusPending.current || editing || depEditing) return
    focusPending.current = false
    focusRow(current)
  }, [current, editing, depEditing, focusRow])

  const commit = useCallback(
    (id: string, span: Span) => {
      const it = byId.get(id)
      if (!it || sameSpan(baseSpans.get(id), span)) {
        setDraft(null)
        return
      }
      setDraft({ id, span, committing: true })
      mutations.setValue.mutate(
        { itemId: id, columnId: timelineCol.id, nilai: { mulai: isoOf(span.start), selesai: isoOf(span.end) } },
        // Draf dipertahankan sampai cache memuat nilai baru (atau dikembalikan bila gagal).
        { onSettled: () => setDraft((d) => (d?.id === id && d.committing && sameSpan(d.span, span) ? null : d)) },
      )
      setAnnounce(`Jadwal ${it.judul}: ${describeSpan(span)}`)
    },
    [byId, baseSpans, mutations.setValue, timelineCol.id],
  )

  const flushKeyDraft = useCallback(() => {
    window.clearTimeout(keyTimer.current)
    keyTimer.current = undefined
    if (draft && !draft.committing && !dragRef.current) commit(draft.id, draft.span)
  }, [draft, commit])

  useEffect(() => () => window.clearTimeout(keyTimer.current), [])

  function shiftByKey(it: Item, mode: DragMode, delta: number) {
    const base = draft?.id === it.id ? draft.span : baseSpans.get(it.id)
    if (!base || !it.bisa_ubah) return
    const span = applyDrag(base, mode, delta)
    setDraft({ id: it.id, span })
    setAnnounce(describeSpan(span))
    window.clearTimeout(keyTimer.current)
    keyTimer.current = window.setTimeout(() => {
      keyTimer.current = undefined
      commit(it.id, span)
    }, KEY_COMMIT_MS)
  }

  function onRowKeyDown(e: KeyboardEvent<HTMLElement>, index: number) {
    const row = rows[index]
    if (!row) return
    const go = (i: number) => {
      e.preventDefault()
      flushKeyDraft()
      moveTo(i)
    }
    switch (e.key) {
      case 'ArrowDown':
        return go(index + 1)
      case 'ArrowUp':
        return go(index - 1)
      case 'Home':
        return e.ctrlKey ? go(0) : undefined
      case 'End':
        return e.ctrlKey ? go(rows.length - 1) : undefined
    }
    if (row.kind === 'group') {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault()
        toggleGroup(row.group.id)
      }
      return
    }
    if (row.kind !== 'item') return
    const it = row.item
    if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
      e.preventDefault()
      shiftByKey(it, e.shiftKey ? 'selesai' : 'geser', e.key === 'ArrowLeft' ? -1 : 1)
    } else if (e.key === 'Escape' && draft?.id === it.id) {
      e.preventDefault()
      window.clearTimeout(keyTimer.current)
      keyTimer.current = undefined
      setDraft(null)
      setAnnounce('Perubahan jadwal dibatalkan.')
    } else if ((e.key === 'Enter' || e.key === 'F2') && it.bisa_ubah) {
      e.preventDefault()
      flushKeyDraft()
      setEditing({ id: it.id, anchor: e.currentTarget })
    } else if ((e.key === 'd' || e.key === 'D') && !e.ctrlKey && !e.metaKey && !e.altKey) {
      e.preventDefault()
      flushKeyDraft()
      setDepEditing({ id: it.id, anchor: e.currentTarget })
    }
  }

  function openDeps(it: Item, anchor: HTMLElement) {
    flushKeyDraft()
    setDepEditing({ id: it.id, anchor })
  }

  function toggleGroup(groupId: string) {
    setCollapsed((s) => {
      const next = new Set(s)
      if (next.has(groupId)) next.delete(groupId)
      else next.add(groupId)
      return next
    })
  }

  function onPointerDown(e: PointerEvent<HTMLElement>, it: Item, index: number) {
    setActiveRow(index)
    const base = baseSpans.get(it.id)
    if (!base || !it.bisa_ubah || e.button !== 0) return
    e.preventDefault()
    e.currentTarget.focus({ preventScroll: true })
    flushKeyDraft()
    const mode = ((e.target as HTMLElement).dataset.mode as DragMode | undefined) ?? 'geser'
    e.currentTarget.setPointerCapture?.(e.pointerId)
    dragRef.current = { id: it.id, mode, startX: e.clientX, base, delta: 0 }
  }

  function onPointerMove(e: PointerEvent<HTMLElement>) {
    const d = dragRef.current
    if (!d) return
    const delta = daysFromPx(e.clientX - d.startX, dayW)
    if (delta === d.delta) return
    d.delta = delta
    setDraft({ id: d.id, span: applyDrag(d.base, d.mode, delta) })
  }

  function onPointerUp() {
    const d = dragRef.current
    if (!d) return
    dragRef.current = null
    if (d.delta === 0) {
      setDraft(null)
      return
    }
    commit(d.id, applyDrag(d.base, d.mode, d.delta))
  }

  function onPointerCancel() {
    dragRef.current = null
    setDraft(null)
  }

  const editItem = editing ? byId.get(editing.id) : undefined
  const total = countItems(rows)
  const todayX = xOf(today, range, dayW)
  const todayVisible = today >= range.start && today < range.end
  const virtualRows = virtualizer.getVirtualItems()
  const viewTop = (virtualRows[0]?.start ?? headerH) - headerH
  const viewBottom = (virtualRows[virtualRows.length - 1]?.end ?? headerH) - headerH
  const visibleArrows = arrows.filter((a) => a.bottom >= viewTop && a.top <= viewBottom)
  const bodyH = virtualizer.getTotalSize()
  const violated = arrows.some((a) => a.violated)
  const anyCritical = schedule.data?.items.some((s) => s.kritis && !s.tanpa_jadwal) ?? false
  const anyLate = schedule.data?.items.some((s) => s.melanggar_fs) ?? false
  const depItem = depEditing ? byId.get(depEditing.id) : undefined

  return (
    <div className="gap-d flex flex-col">
      {depsCtx.loadError != null && <Alert>Dependensi gagal dimuat: {errorMessage(depsCtx.loadError)}</Alert>}
      {schedule.isError && <Alert>Jalur kritis gagal dihitung: {errorMessage(schedule.error)}</Alert>}
      {schedule.data?.peringatan.map((w) => (
        <Alert key={`${w.kode}:${w.tahun ?? ''}`} kind="ok">
          {w.pesan}
        </Alert>
      ))}
      {primaryCol && primaryCol.id !== timelineCol.id && (
        <p role="note" className="text-muted-foreground">
          Pergeseran otomatis successor tidak berlaku pada kolom {timelineCol.judul}. Pergeseran hanya terjadi saat
          jadwal di kolom timeline utama ({primaryCol.judul}) berubah.
        </p>
      )}
      <p id={helpId} className="sr-only">
        Panah atas dan bawah berpindah baris. Panah kiri dan kanan menggeser jadwal satu hari; Shift dengan panah
        mengubah tanggal selesai. Enter membuka editor tanggal. D membuka dependensi item. Esc membatalkan perubahan yang
        belum disimpan.
      </p>
      <div aria-live="polite" className="sr-only">
        {announce}
      </div>
      <div
        ref={scrollRef}
        role="grid"
        aria-label={`Gantt ${board.nama}, ${total} item`}
        aria-rowcount={rows.length + 1}
        aria-colcount={2}
        data-testid="gantt-scroll"
        className="board-scroll relative rounded border border-line bg-surface"
        style={{ '--gantt-day': `var(--gantt-day-${scale})` } as CSSProperties}
      >
        <div role="row" aria-rowindex={1} className="gantt-header sticky top-0 z-20 flex w-max border-b border-line bg-surface">
          <div
            role="columnheader"
            className="px-d sticky left-0 z-30 flex w-(--gantt-list-w) shrink-0 items-end border-r border-line bg-surface pb-(--space-sm) font-semibold"
          >
            Item
          </div>
          <div role="columnheader" aria-label="Linimasa" className="relative shrink-0" style={{ width: totalW }}>
            {[ticks.top, ticks.bottom].map((tier, t) => (
              <div key={t} aria-hidden="true" className={`gantt-tier relative ${t === 0 ? 'border-b border-line font-semibold' : ''}`}>
                {tier.map((tk) => (
                  <span
                    key={tk.key}
                    title={tk.title}
                    className="px-d absolute top-0 flex h-full items-center truncate border-l border-line text-muted-foreground"
                    style={{ left: tk.left, width: tk.width }}
                  >
                    {tk.label}
                  </span>
                ))}
              </div>
            ))}
            {todayVisible && (
              <span
                className="gantt-today absolute top-0 bottom-0 bg-destructive"
                style={{ left: todayX }}
                title="Hari ini"
                aria-label="Hari ini"
                role="img"
              />
            )}
          </div>
        </div>

        <div className="relative w-max" style={{ height: bodyH }}>
          <div
            aria-hidden="true"
            className={`absolute top-0 ${scale === 'bulan' ? '' : 'gantt-weekends'}`}
            style={{ left: listW, width: totalW, height: bodyH }}
          >
            {todayVisible && <div className="gantt-today absolute top-0 bottom-0 bg-destructive" style={{ left: todayX }} />}
          </div>
          <svg
            aria-hidden="true"
            data-testid="gantt-arrows"
            className="pointer-events-none absolute top-0 overflow-visible"
            style={{ left: listW }}
            width={totalW}
            height={bodyH}
          >
            <defs>
              <marker id="gantt-head" viewBox="0 0 6 6" refX="6" refY="3" markerWidth="6" markerHeight="6" orient="auto">
                <path d="M0 0L6 3L0 6z" className="fill-muted-foreground" />
              </marker>
              <marker id="gantt-head-violated" viewBox="0 0 6 6" refX="6" refY="3" markerWidth="6" markerHeight="6" orient="auto">
                <path d="M0 0L6 3L0 6z" className="fill-destructive" />
              </marker>
            </defs>
            {visibleArrows.map((a) => (
              <path
                key={a.id}
                d={a.d}
                data-violated={a.violated || undefined}
                className={`gantt-arrow ${a.violated ? 'text-destructive' : 'text-muted-foreground'}`}
                strokeDasharray={a.violated ? '4 3' : undefined}
                markerEnd={a.violated ? 'url(#gantt-head-violated)' : 'url(#gantt-head)'}
              />
            ))}
          </svg>

          {virtualRows.map((vr) => {
            const row = rows[vr.index]
            if (!row) return null
            return (
              <GanttRow
                key={vr.key}
                row={row}
                index={vr.index}
                top={vr.start - headerH}
                totalW={totalW}
                span={row.kind === 'item' ? spans.get(row.item.id) : undefined}
                sched={row.kind === 'item' ? schedById.get(row.item.id) : undefined}
                onOpenDeps={openDeps}
                range={range}
                dayW={dayW}
                todayX={Math.max(0, todayX)}
                tabbable={vr.index === current}
                helpId={helpId}
                onKeyDown={onRowKeyDown}
                onToggle={toggleGroup}
                onFocusRow={setActiveRow}
                onPointerDown={onPointerDown}
                onPointerMove={onPointerMove}
                onPointerUp={onPointerUp}
                onPointerCancel={onPointerCancel}
                onBlurBar={flushKeyDraft}
              />
            )
          })}
        </div>
      </div>
      <p className="gap-d flex flex-wrap text-muted-foreground">
        <span>
          Seret batang untuk menggeser jadwal; seret ujungnya untuk mengubah tanggal mulai atau selesai. Tekan D untuk
          mengatur dependensi.
        </span>
        {anyCritical && <span>Batang merah: jalur kritis (slack 0 hari kerja).</span>}
        {(violated || anyLate) && (
          <span className="text-destructive">
            Garis putus-putus merah: item dijadwalkan mulai sebelum predecessor selesai (termasuk lag).
          </span>
        )}
        {schedule.data?.selesai_proyek && <span>Perkiraan selesai: {formatDate(schedule.data.selesai_proyek)}.</span>}
      </p>
      {depEditing && depItem && (
        <DependencyEditor
          item={depItem}
          anchor={depEditing.anchor}
          onClose={() => {
            setDepEditing(null)
            focusPending.current = true
          }}
        />
      )}
      {editing && editItem && (
        <CellEditor
          col={timelineCol}
          item={editItem}
          anchor={editing.anchor}
          label={`${timelineCol.judul} item ${editItem.judul}`}
          members={[]}
          onInvalid={onError}
          onCancel={() => {
            setEditing(null)
            focusPending.current = true
          }}
          onCommit={(value) => {
            setEditing(null)
            focusPending.current = true
            mutations.setValue.mutate({ itemId: editItem.id, columnId: timelineCol.id, nilai: value })
          }}
        />
      )}
    </div>
  )
}

function GanttRow({
  row,
  index,
  top,
  totalW,
  span,
  sched,
  onOpenDeps,
  range,
  dayW,
  todayX,
  tabbable,
  helpId,
  onKeyDown,
  onToggle,
  onFocusRow,
  onPointerDown,
  onPointerMove,
  onPointerUp,
  onPointerCancel,
  onBlurBar,
}: {
  row: GridRow
  index: number
  top: number
  totalW: number
  span: Span | undefined
  /** Hasil CPM item (kritis, slack, melanggar FS) dari backend. */
  sched: ItemSchedule | undefined
  onOpenDeps: (it: Item, anchor: HTMLElement) => void
  range: Span
  dayW: number
  todayX: number
  tabbable: boolean
  helpId: string
  onKeyDown: (e: KeyboardEvent<HTMLElement>, index: number) => void
  onToggle: (groupId: string) => void
  onFocusRow: (index: number) => void
  onPointerDown: (e: PointerEvent<HTMLElement>, it: Item, index: number) => void
  onPointerMove: (e: PointerEvent<HTMLElement>) => void
  onPointerUp: () => void
  onPointerCancel: () => void
  onBlurBar: () => void
}) {
  const rowStyle = { transform: `translateY(${top}px)` }
  if (row.kind === 'group') {
    const { group } = row
    return (
      <div role="row" aria-rowindex={index + 2} className="gantt-row absolute top-0 left-0 w-max border-b border-line" style={rowStyle}>
        <div role="gridcell" className={`tag-${group.warna} sticky left-0 z-10 w-(--gantt-list-w) shrink-0 border-r border-line bg-canvas`}>
          <div
            role="button"
            aria-expanded={!row.collapsed}
            aria-label={`Grup ${group.judul}, ${row.count} item`}
            data-gantt-row={index}
            tabIndex={tabbable ? 0 : -1}
            onFocus={() => onFocusRow(index)}
            onClick={() => onToggle(group.id)}
            onKeyDown={(e) => onKeyDown(e, index)}
            className="tag-bar px-d gap-d flex h-full cursor-pointer items-center font-semibold outline-none focus-visible:ring-(length:--focus-ring-w) focus-visible:ring-primary focus-visible:ring-inset"
          >
            <span aria-hidden="true" className="inline-block w-(--icon-size) text-muted-foreground">
              {row.collapsed ? '▸' : '▾'}
            </span>
            <span className="truncate" style={{ color: 'var(--tag)' }}>
              {group.judul}
            </span>
            <span className="font-normal text-muted-foreground">{row.count} item</span>
          </div>
        </div>
        <div role="gridcell" className="shrink-0" style={{ width: totalW }} />
      </div>
    )
  }
  if (row.kind !== 'item') return null
  const it = row.item
  const box = span ? barBox(span, range, dayW) : null
  const focusProps = {
    'data-gantt-row': index,
    tabIndex: tabbable ? 0 : -1,
    onFocus: () => onFocusRow(index),
    onKeyDown: (e: KeyboardEvent<HTMLElement>) => onKeyDown(e, index),
    'aria-describedby': helpId,
  }
  const ring = 'outline-none focus-visible:ring-(length:--focus-ring-w) focus-visible:ring-ring focus-visible:ring-offset-(length:--line-w)'
  const access = it.bisa_ubah ? '' : ', hanya baca'
  const kritis = !!sched?.kritis
  const late = !!sched?.melanggar_fs
  const cpm = `${kritis ? ', jalur kritis' : sched?.slack != null ? `, slack ${sched.slack} hari kerja` : ''}${
    late ? ', mulai sebelum predecessor selesai' : ''
  }`
  return (
    <div role="row" aria-rowindex={index + 2} className="gantt-row absolute top-0 left-0 w-max border-b border-line" style={rowStyle}>
      <div
        role="gridcell"
        title={it.judul}
        className="px-d gap-d sticky left-0 z-10 flex w-(--gantt-list-w) shrink-0 items-center justify-between border-r border-line bg-surface"
      >
        <span className="truncate">{it.judul}</span>
        {/* Di luar urutan Tab (roving tabindex baris); keyboard memakai tombol D. */}
        <button
          type="button"
          tabIndex={-1}
          aria-label={`Dependensi ${it.judul}`}
          title="Dependensi (D)"
          onClick={(e) => onOpenDeps(it, e.currentTarget)}
          className="shrink-0 rounded px-(--space-sm) text-muted-foreground hover:bg-muted hover:text-foreground"
        >
          <svg aria-hidden="true" viewBox="0 0 16 16" className="size-(--icon-size) fill-none stroke-current">
            <path d="M2 4h6v8h6M11 9l3 3-3 3" strokeWidth="1.5" />
          </svg>
        </button>
      </div>
      <div role="gridcell" className="relative shrink-0" style={{ width: totalW }}>
        {box && span ? (
          <div
            {...focusProps}
            role="button"
            data-left={box.left}
            data-width={box.width}
            aria-label={`${it.judul}: ${describeSpan(span)}${cpm}${access}`}
            data-kritis={kritis || undefined}
            data-violates={late || undefined}
            onPointerDown={(e) => onPointerDown(e, it, index)}
            onPointerMove={onPointerMove}
            onPointerUp={onPointerUp}
            onPointerCancel={onPointerCancel}
            onBlur={onBlurBar}
            className={`gantt-bar px-d absolute flex touch-none items-center overflow-hidden rounded text-(length:--font-xs) select-none ${ring} ${
              kritis ? 'bg-destructive text-primary-foreground' : it.bisa_ubah ? 'bg-primary text-primary-foreground' : 'bg-muted-foreground text-primary-foreground'
            } ${it.bisa_ubah ? 'cursor-grab' : 'cursor-default'}`}
            style={{ left: box.left, width: box.width }}
          >
            {it.bisa_ubah && <span aria-hidden="true" data-mode="mulai" className="gantt-handle absolute top-0 bottom-0 left-0" />}
            <span className="truncate">{it.judul}</span>
            {it.bisa_ubah && <span aria-hidden="true" data-mode="selesai" className="gantt-handle absolute top-0 right-0 bottom-0" />}
          </div>
        ) : (
          <div
            {...focusProps}
            role="button"
            aria-label={`${it.judul}: belum dijadwalkan${access}`}
            className={`gantt-bar px-d absolute flex items-center rounded border border-dashed border-line text-(length:--font-xs) text-muted-foreground ${ring}`}
            style={{ left: todayX }}
          >
            Belum dijadwalkan
          </div>
        )}
      </div>
    </div>
  )
}
