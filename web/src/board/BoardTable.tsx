import { createColumnHelper, flexRender, metaHelper, tableFeatures, useTable, type Row } from '@tanstack/react-table'
import { useVirtualizer } from '@tanstack/react-virtual'
import {
  createContext,
  memo,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent,
  type ReactNode,
} from 'react'

import type { BoardDetail, Column, Item, Member } from '../api/types'
import { cssPx, useDensity } from '../density'
import { OptionList, Popover, type Option } from './cells/Popover'
import { CellDisplay } from './cells/display'
import { acceptsTyping, CellEditor, isInlineEditor, type Move } from './cells/editors'
import { buildRows, countItems, type GridRow } from './rows'
import type { ItemMutations } from './useBoardItems'

// Tabel board padat: satu area gulir, baris tinggi tetap yang divirtualisasi,
// header menempel di atas dan kolom judul menempel di kiri. Kolom 0 = judul,
// 1..n = kolom board, n+1 = aksi.

type ColMeta = { kind: 'title' | 'value' | 'actions'; col?: Column; width: string }

const features = tableFeatures({ columnMeta: metaHelper<ColMeta>() })
const helper = createColumnHelper<typeof features, GridRow>()

const getRowId = (r: GridRow) => r.key
// Tabel tidak memakai state fitur; jangan berlangganan perubahan state.
const noState = () => null

const NamesContext = createContext<ReadonlyMap<string, string>>(new Map())

function ValueDisplay({ col, item }: { col: Column; item: Item }) {
  return <CellDisplay col={col} item={item} names={useContext(NamesContext)} />
}

function buildColumns(kolom: Column[]) {
  return helper.columns([
    helper.display({
      id: '_judul',
      header: 'Item',
      meta: { kind: 'title', width: '--col-w-title' },
      cell: ({ row }) =>
        row.original.kind === 'item' ? <span className="px-d flex h-full items-center truncate">{row.original.item.judul}</span> : null,
    }),
    ...kolom.map((c) =>
      helper.display({
        id: c.id,
        header: c.judul,
        meta: { kind: 'value', col: c, width: `--col-w-${c.tipe}` },
        cell: ({ row }) => (row.original.kind === 'item' ? <ValueDisplay col={c} item={row.original.item} /> : null),
      }),
    ),
    helper.display({
      id: '_aksi',
      header: '',
      meta: { kind: 'actions', width: '--col-w-actions' },
      cell: ({ row }) =>
        row.original.kind === 'item' && (row.original.item.bisa_ubah || row.original.item.bisa_hapus) ? (
          <span aria-hidden="true" className="flex h-full items-center justify-center text-muted-foreground">
            ⋯
          </span>
        ) : null,
    }),
  ])
}

type Pos = { r: number; c: number }
type Editing = Pos & { initial?: string; anchor: HTMLElement }

/** Aksi tabel yang dipakai baris; objeknya stabil agar baris memo tidak dirender ulang. */
interface GridActions {
  activate: (p: Pos) => void
  startEdit: (p: Pos, initial?: string) => void
  toggleGroup: (groupId: string) => void
  create: (groupId: string, judul: string) => void
}

const ActionsContext = createContext<GridActions | null>(null)

function useActions(): GridActions {
  const a = useContext(ActionsContext)
  if (!a) throw new Error('ActionsContext belum dipasang')
  return a
}

function cellEditable(row: GridRow, meta: ColMeta): boolean {
  if (row.kind !== 'item') return false
  const it = row.item
  if (meta.kind === 'actions') return it.bisa_ubah || it.bisa_hapus
  if (meta.kind === 'value' && meta.col?.tipe === 'dependency') return false
  return it.bisa_ubah
}

const STICKY = 'sticky left-0 z-10'

const ItemRow = memo(function ItemRow({
  row,
  index,
  start,
  activeCol,
  editingCol,
}: {
  row: Row<typeof features, GridRow>
  index: number
  start: number
  activeCol: number
  editingCol: number
}) {
  const actions = useActions()
  const r = row.original
  if (r.kind !== 'item') return null
  const label = r.item.judul
  return (
    <div
      role="row"
      aria-rowindex={index + 2}
      className="board-row group absolute top-0 left-0 border-b border-line bg-surface hover:bg-canvas"
      style={{ transform: `translateY(${start}px)` }}
    >
      {row.getAllCells().map((cell, c) => {
        const meta = cell.column.columnDef.meta as ColMeta
        const editable = cellEditable(r, meta)
        const active = c === activeCol
        const editing = c === editingCol
        const header = meta.kind === 'title' ? 'Judul' : meta.kind === 'actions' ? 'Aksi' : (meta.col?.judul ?? '')
        return (
          <div
            key={cell.id}
            role="gridcell"
            aria-colindex={c + 1}
            aria-label={meta.kind === 'actions' ? `Aksi item ${label}` : undefined}
            aria-readonly={meta.kind !== 'actions' && !editable ? true : undefined}
            aria-haspopup={meta.kind === 'actions' && editable ? 'menu' : undefined}
            title={meta.kind === 'title' ? label : undefined}
            data-r={index}
            data-c={c}
            data-header={header}
            tabIndex={active ? 0 : -1}
            onMouseDown={() => actions.activate({ r: index, c })}
            onClick={() => {
              if (meta.kind === 'actions' || (meta.kind === 'value' && meta.col && !isInlineEditor(meta.col))) {
                if (editable) actions.startEdit({ r: index, c })
              }
            }}
            onDoubleClick={() => {
              if (editable) actions.startEdit({ r: index, c })
            }}
            className={`board-cell relative border-r border-line outline-none ${
              meta.kind === 'title' ? `${STICKY} bg-inherit` : ''
            } ${active ? 'ring-(length:--focus-ring-w) ring-primary ring-inset' : ''} ${editable ? 'cursor-default' : ''}`}
          >
            {editing && isInlineEditor(meta.kind === 'title' ? null : (meta.col ?? null)) && meta.kind !== 'actions' ? (
              <EditorSlot />
            ) : (
              flexRender(cell.column.columnDef.cell, cell.getContext())
            )}
          </div>
        )
      })}
    </div>
  )
})

// Editor inline dirender lewat konteks agar ItemRow tidak perlu tahu propsnya.
const EditorContext = createContext<ReactNode>(null)
function EditorSlot() {
  return <>{useContext(EditorContext)}</>
}

function GroupRow({
  r,
  index,
  start,
  active,
  ncols,
}: {
  r: Extract<GridRow, { kind: 'group' }>
  index: number
  start: number
  active: boolean
  ncols: number
}) {
  const actions = useActions()
  const { group } = r
  return (
    <div
      role="row"
      aria-rowindex={index + 2}
      className={`board-row tag-${group.warna} absolute top-0 left-0 border-b border-line bg-canvas`}
      style={{ transform: `translateY(${start}px)` }}
    >
      <div role="gridcell" aria-colspan={ncols} style={{ gridColumn: '1 / -1' }} className="board-cell">
        <div
          role="button"
          aria-expanded={!r.collapsed}
          aria-label={`Grup ${group.judul}, ${r.count} item`}
          data-r={index}
          data-c={0}
          tabIndex={active ? 0 : -1}
          onMouseDown={() => actions.activate({ r: index, c: 0 })}
          onClick={() => actions.toggleGroup(group.id)}
          className={`${STICKY} tag-bar px-d gap-d inline-flex h-full cursor-pointer items-center bg-canvas font-semibold outline-none ${
            active ? 'ring-(length:--focus-ring-w) ring-primary ring-inset' : ''
          }`}
        >
          <span aria-hidden="true" className="inline-block w-(--icon-size) text-muted-foreground">
            {r.collapsed ? '▸' : '▾'}
          </span>
          <span style={{ color: 'var(--tag)' }}>{group.judul}</span>
          <span className="font-normal text-muted-foreground">{r.count} item</span>
        </div>
      </div>
    </div>
  )
}

function AddRow({
  r,
  index,
  start,
  active,
  ncols,
}: {
  r: Extract<GridRow, { kind: 'add' }>
  index: number
  start: number
  active: boolean
  ncols: number
}) {
  const actions = useActions()
  const [judul, setJudul] = useState('')
  return (
    <div
      role="row"
      aria-rowindex={index + 2}
      className="board-row absolute top-0 left-0 border-b border-line bg-surface"
      style={{ transform: `translateY(${start}px)` }}
    >
      <div role="gridcell" aria-colspan={ncols} style={{ gridColumn: '1 / -1' }} className="board-cell">
        <input
          aria-label={`Item baru di ${r.group.judul}`}
          placeholder="+ Tambah item"
          data-r={index}
          data-c={0}
          tabIndex={active ? 0 : -1}
          maxLength={500}
          value={judul}
          onFocus={() => actions.activate({ r: index, c: 0 })}
          onChange={(e) => setJudul(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && judul.trim()) {
              e.preventDefault()
              actions.create(r.group.id, judul.trim())
              setJudul('')
            } else if (e.key === 'Escape') {
              setJudul('')
            }
          }}
          className={`${STICKY} board-cell px-d bg-surface outline-none ${active ? 'ring-(length:--focus-ring-w) ring-primary ring-inset' : ''}`}
          style={{ width: 'var(--col-w-title)' }}
        />
      </div>
    </div>
  )
}

function ActionsMenu({
  item,
  board,
  anchor,
  onDone,
  mutations,
}: {
  item: Item
  board: BoardDetail
  anchor: HTMLElement
  onDone: () => void
  mutations: ItemMutations
}) {
  const options: Option[] = []
  if (item.bisa_ubah) {
    for (const g of board.grup) {
      if (g.id !== item.group_id) options.push({ value: `move:${g.id}`, label: `Pindah ke ${g.judul}` })
    }
  }
  if (item.bisa_hapus) options.push({ value: 'delete', label: 'Hapus item' })
  const label = `Aksi item ${item.judul}`
  return (
    <Popover anchor={anchor} label={label} onClose={onDone}>
      <OptionList
        label={label}
        options={options}
        selected=""
        onCancel={onDone}
        onSelect={(v) => {
          onDone()
          if (v === 'delete') {
            if (window.confirm(`Hapus "${item.judul}"?`)) mutations.remove.mutate({ itemId: item.id })
          } else if (v.startsWith('move:')) {
            mutations.move.mutate({ itemId: item.id, groupId: v.slice(5) })
          }
        }}
      />
    </Popover>
  )
}

function sameValue(a: unknown, b: unknown): boolean {
  return JSON.stringify(a ?? null) === JSON.stringify(b ?? null)
}

export function BoardTable({
  board,
  items,
  members,
  canCreate,
  mutations,
  onError,
}: {
  board: BoardDetail
  items: Item[]
  members: Member[]
  canCreate: boolean
  mutations: ItemMutations
  onError: (message: string | null) => void
}) {
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(() => new Set())
  const [active, setActive] = useState<Pos>({ r: 0, c: 0 })
  const [editing, setEditing] = useState<Editing | null>(null)
  const scrollRef = useRef<HTMLDivElement>(null)
  const focusPending = useRef(false)

  const kolom = useMemo(() => [...board.kolom].sort((a, b) => a.urutan - b.urutan), [board.kolom])
  const columns = useMemo(() => buildColumns(kolom), [kolom])
  const rows = useMemo(() => buildRows(board.grup, items, collapsed, canCreate), [board.grup, items, collapsed, canCreate])
  const names = useMemo(() => new Map(members.map((m) => [m.user_id, m.nama])), [members])
  const ncols = kolom.length + 2

  // Opsi tabel harus stabil antar-render (tiap render terjadi saat menggulir).
  const tableOptions = useMemo(() => ({ features, columns, data: rows, getRowId }), [columns, rows])
  const table = useTable(tableOptions, noState)
  const tableRows = table.getRowModel().rows

  const density = useDensity()
  // eslint-disable-next-line react-hooks/exhaustive-deps -- dibaca ulang saat kepadatan berganti
  const rowPx = useMemo(() => cssPx('--density-row', 32), [density])

  // Proyek ini tidak memakai React Compiler; peringatan memoisasi tidak berlaku.
  // eslint-disable-next-line react-hooks/incompatible-library
  const virtualizer = useVirtualizer({
    count: tableRows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => rowPx,
    getItemKey: (i) => tableRows[i]?.id ?? i,
    // Cadangan kecil: lompatan jauh (seret scrollbar) memasang lebih sedikit baris.
    overscan: 6,
    scrollMargin: rowPx,
    scrollPaddingStart: rowPx,
  })

  useEffect(() => {
    virtualizer.measure()
  }, [rowPx, virtualizer])

  // Posisi aktif tetap dalam batas saat baris berkurang (hapus, lipat grup).
  const activeRow = Math.min(active.r, Math.max(0, rows.length - 1))
  const activeKind = rows[activeRow]?.kind
  const activeColEff = activeKind === 'item' ? Math.min(active.c, ncols - 1) : 0

  const focusCell = useCallback((p: Pos, tries = 6) => {
    const sc = scrollRef.current
    const el = sc?.querySelector<HTMLElement>(`[data-r="${p.r}"][data-c="${p.c}"]`)
    if (!sc || !el) {
      if (tries > 0) requestAnimationFrame(() => focusCell(p, tries - 1))
      return
    }
    if (p.c > 0) {
      const sticky = sc.querySelector<HTMLElement>(`[data-r="${p.r}"][data-c="0"]`)?.offsetWidth ?? 0
      const left = el.offsetLeft
      if (left - sticky < sc.scrollLeft) sc.scrollLeft = left - sticky
      else if (left + el.offsetWidth > sc.scrollLeft + sc.clientWidth) sc.scrollLeft = left + el.offsetWidth - sc.clientWidth
    }
    el.focus({ preventScroll: true })
  }, [])

  const moveTo = useCallback(
    (p: Pos) => {
      const r = Math.max(0, Math.min(rows.length - 1, p.r))
      const c = Math.max(0, Math.min(ncols - 1, p.c))
      // Tandai dulu: scrollToIndex dapat memicu render sinkron (flushSync) yang
      // menjalankan efek fokus sebelum baris berikutnya.
      focusPending.current = true
      setActive({ r, c })
      virtualizer.scrollToIndex(r, { align: 'auto' })
    },
    [rows.length, ncols, virtualizer],
  )

  useEffect(() => {
    if (!focusPending.current || editing) return
    focusPending.current = false
    focusCell({ r: activeRow, c: activeColEff })
  }, [activeRow, activeColEff, editing, focusCell])

  const startEdit = useCallback(
    (p: Pos, initial?: string) => {
      const row = rows[p.r]
      const meta = columns[p.c]?.meta as ColMeta | undefined
      if (!row || !meta || !cellEditable(row, meta)) return
      const anchor = scrollRef.current?.querySelector<HTMLElement>(`[data-r="${p.r}"][data-c="${p.c}"]`)
      if (!anchor) return
      setActive(p)
      setEditing({ ...p, initial, anchor })
    },
    [rows, columns],
  )

  const finishEdit = useCallback(
    (move: Move) => {
      if (!editing) return
      setEditing(null)
      const delta: Record<Exclude<Move, null>, Pos> = {
        down: { r: editing.r + 1, c: editing.c },
        right: { r: editing.r, c: editing.c + 1 },
        left: { r: editing.r, c: editing.c - 1 },
      }
      moveTo(move ? delta[move] : { r: editing.r, c: editing.c })
    },
    [editing, moveTo],
  )

  const commit = useCallback(
    (p: Pos, value: unknown) => {
      const row = rows[p.r]
      if (row?.kind !== 'item') return
      const it = row.item
      if (p.c === 0) {
        if (typeof value === 'string' && value !== it.judul) mutations.setTitle.mutate({ itemId: it.id, judul: value })
        return
      }
      const col = kolom[p.c - 1]
      if (!col || sameValue(it.nilai[col.id], value)) return
      mutations.setValue.mutate({ itemId: it.id, columnId: col.id, nilai: value })
    },
    [rows, kolom, mutations],
  )

  // Objek aksi stabil; isinya selalu memakai fungsi terbaru.
  const latest = useRef({ moveTo, startEdit, create: mutations.create.mutate })
  useEffect(() => {
    latest.current = { moveTo, startEdit, create: mutations.create.mutate }
  })
  const gridActions = useMemo<GridActions>(
    () => ({
      activate: (p) => setActive(p),
      startEdit: (p, initial) => latest.current.startEdit(p, initial),
      toggleGroup: (groupId) =>
        setCollapsed((s) => {
          const next = new Set(s)
          if (next.has(groupId)) next.delete(groupId)
          else next.add(groupId)
          return next
        }),
      create: (groupId, judul) => latest.current.create({ groupId, judul }),
    }),
    [],
  )

  function onKeyDown(e: KeyboardEvent<HTMLDivElement>) {
    if (editing) return
    const target = e.target as HTMLElement
    const inInput = target.tagName === 'INPUT'
    if (inInput && e.key !== 'ArrowUp' && e.key !== 'ArrowDown') return
    const row = rows[activeRow]
    const pageRows = Math.max(1, Math.floor((scrollRef.current?.clientHeight ?? rowPx * 10) / rowPx) - 2)
    const go = (p: Pos) => {
      e.preventDefault()
      moveTo(p)
    }
    const c = activeColEff
    switch (e.key) {
      case 'ArrowDown':
        return go({ r: activeRow + 1, c: active.c })
      case 'ArrowUp':
        return go({ r: activeRow - 1, c: active.c })
      case 'ArrowRight':
        return row?.kind === 'item' ? go({ r: activeRow, c: c + 1 }) : undefined
      case 'ArrowLeft':
        return row?.kind === 'item' ? go({ r: activeRow, c: c - 1 }) : undefined
      case 'PageDown':
        return go({ r: activeRow + pageRows, c: active.c })
      case 'PageUp':
        return go({ r: activeRow - pageRows, c: active.c })
      case 'Home':
        return go(e.ctrlKey ? { r: 0, c: 0 } : { r: activeRow, c: 0 })
      case 'End':
        return go(e.ctrlKey ? { r: rows.length - 1, c: active.c } : { r: activeRow, c: ncols - 1 })
    }
    if (!row) return
    if (row.kind === 'group' && (e.key === 'Enter' || e.key === ' ')) {
      e.preventDefault()
      gridActions.toggleGroup(row.group.id)
      return
    }
    if (row.kind !== 'item') return
    const meta = columns[c]?.meta as ColMeta | undefined
    if (!meta) return
    if (e.key === 'Enter' || e.key === 'F2') {
      e.preventDefault()
      startEdit({ r: activeRow, c })
      return
    }
    const col = meta.kind === 'title' ? null : (meta.col ?? null)
    if ((e.key === 'Delete' || e.key === 'Backspace') && meta.kind === 'value' && col && cellEditable(row, meta)) {
      e.preventDefault()
      commit({ r: activeRow, c }, null)
      return
    }
    const printable = e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey
    if (printable && meta.kind !== 'actions' && acceptsTyping(col)) {
      e.preventDefault()
      startEdit({ r: activeRow, c }, e.key)
    }
  }

  const editRow = editing ? rows[editing.r] : undefined
  const editMeta = editing ? (columns[editing.c]?.meta as ColMeta | undefined) : undefined
  let editor: ReactNode = null
  if (editing && editRow?.kind === 'item' && editMeta) {
    const it = editRow.item
    if (editMeta.kind === 'actions') {
      editor = <ActionsMenu item={it} board={board} anchor={editing.anchor} mutations={mutations} onDone={() => finishEdit(null)} />
    } else {
      const col = editMeta.kind === 'title' ? null : (editMeta.col ?? null)
      editor = (
        <CellEditor
          key={`${editing.r}:${editing.c}`}
          col={col}
          item={it}
          anchor={editing.anchor}
          label={`${col ? col.judul : 'Judul'} item ${it.judul}`}
          initial={editing.initial}
          members={members}
          onInvalid={onError}
          onCancel={() => finishEdit(null)}
          onCommit={(value, move) => {
            commit({ r: editing.r, c: editing.c }, value)
            finishEdit(move)
          }}
        />
      )
    }
  }
  const inlineEditing = editing !== null && editMeta?.kind !== 'actions' && isInlineEditor(editMeta?.kind === 'title' ? null : (editMeta?.col ?? null))

  const gridCols = useMemo(
    () => columns.map((c) => `var(${(c.meta as ColMeta).width})`).join(' '),
    [columns],
  )
  const headers = table.getHeaderGroups()[0]?.headers ?? []
  const total = countItems(rows)

  return (
    <NamesContext.Provider value={names}>
      <ActionsContext.Provider value={gridActions}>
        <EditorContext.Provider value={inlineEditing ? editor : null}>
          <div
            ref={scrollRef}
            role="grid"
            aria-label={`Tabel ${board.nama}, ${total} item`}
            aria-rowcount={rows.length + 1}
            aria-colcount={ncols}
            data-testid="board-scroll"
            className="board-scroll rounded border border-line bg-surface"
            style={{ '--grid-cols': gridCols } as CSSProperties}
            onKeyDown={onKeyDown}
          >
            <div role="row" aria-rowindex={1} className="board-row sticky top-0 z-20 border-b border-line bg-surface font-semibold">
              {headers.map((h, c) => (
                <div
                  key={h.id}
                  role="columnheader"
                  aria-colindex={c + 1}
                  className={`board-cell px-d flex items-center border-r border-line ${c === 0 ? 'sticky left-0 z-30 bg-surface' : ''}`}
                >
                  <span className="truncate">
                    <table.FlexRender header={h} />
                  </span>
                </div>
              ))}
            </div>
            <div role="presentation" className="relative" style={{ height: virtualizer.getTotalSize(), width: 'max-content', minWidth: '100%' }}>
              {virtualizer.getVirtualItems().map((v) => {
                const tr = tableRows[v.index]
                if (!tr) return null
                const r = tr.original
                const start = v.start - virtualizer.options.scrollMargin
                const isActive = v.index === activeRow
                if (r.kind === 'group') {
                  return <GroupRow key={tr.id} r={r} index={v.index} start={start} active={isActive} ncols={ncols} />
                }
                if (r.kind === 'add') {
                  return <AddRow key={tr.id} r={r} index={v.index} start={start} active={isActive} ncols={ncols} />
                }
                return (
                  <ItemRow
                    key={tr.id}
                    row={tr}
                    index={v.index}
                    start={start}
                    activeCol={isActive ? activeColEff : -1}
                    editingCol={editing && editing.r === v.index && inlineEditing ? editing.c : -1}
                  />
                )
              })}
            </div>
          </div>
          {!inlineEditing && editor}
        </EditorContext.Provider>
      </ActionsContext.Provider>
    </NamesContext.Provider>
  )
}
