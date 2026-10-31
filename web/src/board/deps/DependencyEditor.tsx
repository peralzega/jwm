import { useEffect, useId, useMemo, useRef, useState } from 'react'

import type { Item } from '../../api/types'
import { Button } from '../../components/ui'
import { Popover } from '../cells/Popover'
import { useDepsContext, type DependencyLinkView } from './DependencyContext'
import { useDependencyMutations } from './queries'

const MAX_MATCHES = 8
const NO_LINKS: DependencyLinkView[] = []

/**
 * Panel dependensi satu item: daftar predecessor dan successor, hapus, dan
 * tambah predecessor dengan lag (hari kerja). Server menolak siklus,
 * dependensi antar-board, dan item yang tidak boleh diubah pelaku.
 */
export function DependencyEditor({
  item,
  anchor,
  onClose,
}: {
  item: Item
  anchor: HTMLElement
  onClose: () => void
}) {
  const ctx = useDepsContext()
  const { create, remove, errorText } = useDependencyMutations(ctx.boardId)
  const [query, setQuery] = useState('')
  const [lag, setLag] = useState('0')
  const searchRef = useRef<HTMLInputElement>(null)
  const ids = { search: useId(), lag: useId(), results: useId() }
  const label = `Dependensi ${item.judul}`
  const preds = ctx.predecessors.get(item.id) ?? NO_LINKS
  const succs = ctx.successors.get(item.id) ?? NO_LINKS
  const editable = item.bisa_ubah

  useEffect(() => {
    searchRef.current?.focus()
  }, [])

  const matches = useMemo(() => {
    const q = query.trim().toLocaleLowerCase('id-ID')
    if (!q) return []
    const taken = new Set(preds.map((l) => l.dep.predecessor_id))
    const out: Item[] = []
    for (const it of ctx.items) {
      if (it.id === item.id || taken.has(it.id) || !it.bisa_ubah) continue
      if (!it.judul.toLocaleLowerCase('id-ID').includes(q)) continue
      out.push(it)
      if (out.length === MAX_MATCHES) break
    }
    return out
  }, [query, ctx.items, item.id, preds])

  const lagNum = Number(lag)
  const lagValid = lag.trim() !== '' && Number.isInteger(lagNum) && lagNum >= -365 && lagNum <= 365

  function add(pred: Item) {
    if (!lagValid) return
    create.mutate(
      { predecessorId: pred.id, successorId: item.id, lagHari: lagNum },
      {
        onSuccess: () => {
          setQuery('')
          searchRef.current?.focus()
        },
      },
    )
  }

  const list = (title: string, links: DependencyLinkView[], other: 'predecessor' | 'successor') => (
    <section aria-label={title} className="flex flex-col">
      <h3 className="font-semibold">{title}</h3>
      {links.length === 0 && <p className="text-muted-foreground">Tidak ada.</p>}
      <ul className="flex flex-col">
        {links.map((l) => {
          const name = l.other?.judul ?? 'Item terhapus'
          return (
            <li key={l.dep.id} className="row-h gap-d flex items-center justify-between">
              <span className="truncate" title={name}>
                {name}
                {l.dep.lag_hari !== 0 && (
                  <span className="text-muted-foreground"> (lag {l.dep.lag_hari} hari kerja)</span>
                )}
              </span>
              {editable && (
                <Button
                  aria-label={`Hapus dependensi ${other === 'predecessor' ? `dari ${name}` : `ke ${name}`}`}
                  disabled={remove.isPending}
                  onClick={() => remove.mutate({ id: l.dep.id })}
                >
                  Hapus
                </Button>
              )}
            </li>
          )
        })}
      </ul>
    </section>
  )

  return (
    <Popover anchor={anchor} label={label} onClose={onClose}>
      <div
        className="px-d py-d gap-d flex w-(--dep-panel-w) flex-col"
        onKeyDown={(e) => {
          if (e.key === 'Escape') {
            e.preventDefault()
            onClose()
          }
          e.stopPropagation()
        }}
      >
        {list('Bergantung pada (predecessor)', preds, 'predecessor')}
        {list('Diikuti oleh (successor)', succs, 'successor')}
        {editable ? (
          <form
            className="gap-d flex flex-col border-t border-line pt-(--space-sm)"
            onSubmit={(e) => {
              e.preventDefault()
              const first = matches[0]
              if (first) add(first)
            }}
          >
            <label htmlFor={ids.search} className="font-semibold">
              Tambah predecessor
            </label>
            <input
              ref={searchRef}
              id={ids.search}
              type="search"
              placeholder="Cari judul item"
              autoComplete="off"
              aria-controls={ids.results}
              className="row-h px-d rounded border border-line bg-surface"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
            <label htmlFor={ids.lag} className="gap-d flex items-center">
              Lag (hari kerja)
              <input
                id={ids.lag}
                type="number"
                min={-365}
                max={365}
                step={1}
                aria-invalid={!lagValid}
                className="row-h px-d w-(--dep-lag-w) rounded border border-line bg-surface tabular-nums"
                value={lag}
                onChange={(e) => setLag(e.target.value)}
              />
            </label>
            {!lagValid && <p className="text-danger">Lag harus bilangan bulat −365 sampai 365.</p>}
            <ul id={ids.results} aria-label="Hasil pencarian" className="flex flex-col">
              {query.trim() !== '' && matches.length === 0 && (
                <li className="text-muted-foreground">Tidak ada item yang cocok atau boleh Anda ubah.</li>
              )}
              {matches.map((m) => (
                <li key={m.id}>
                  <Button
                    className="w-full justify-start"
                    disabled={create.isPending || !lagValid}
                    aria-label={`Tambahkan ${m.judul} sebagai predecessor`}
                    onClick={() => add(m)}
                  >
                    <span className="truncate">{m.judul}</span>
                  </Button>
                </li>
              ))}
            </ul>
          </form>
        ) : (
          <p className="text-muted-foreground">Anda tidak berhak mengubah dependensi item ini.</p>
        )}
        {(errorText || ctx.loadError != null) && (
          <p role="alert" className="text-danger">
            {errorText ?? 'Dependensi gagal dimuat.'}
          </p>
        )}
        <div className="flex justify-end">
          <Button onClick={onClose}>Tutup</Button>
        </div>
      </div>
    </Popover>
  )
}
