import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router'

import { api, errorMessage } from '../api/client'
import type { BoardDetail, Column, Group, Item, Member, Page } from '../api/types'
import { Alert, Button, Loading } from '../components/ui'
import { NotFoundPage } from './NotFoundPage'

// Daftar item sederhana untuk M1. Tabel padat bervirtualisasi dan Kanban dibuat di M2.

async function fetchAllItems(boardId: string): Promise<Item[]> {
  const all: Item[] = []
  let cursor: string | null | undefined = undefined
  do {
    const qs: string = cursor ? `&kursor=${cursor}` : ''
    const page: Page<Item> = await api<Page<Item>>('GET', `/boards/${boardId}/items?batas=1000${qs}`)
    all.push(...page.items)
    cursor = page.kursor_berikut
  } while (cursor)
  return all
}

/** Mengelompokkan item per grup, masing-masing urut menurut urutan. */
export function groupItems(groups: Group[], items: Item[]): { group: Group; items: Item[] }[] {
  return [...groups]
    .sort((a, b) => a.urutan - b.urutan)
    .map((group) => ({
      group,
      items: items.filter((i) => i.group_id === group.id).sort((a, b) => a.urutan - b.urutan),
    }))
}

function ItemRow({
  item,
  groups,
  statusCol,
  personCol,
  names,
  onError,
}: {
  item: Item
  groups: Group[]
  statusCol?: Column
  personCol?: Column
  names: Map<string, string>
  onError: (msg: string | null) => void
}) {
  const qc = useQueryClient()
  const [judul, setJudul] = useState(item.judul)
  const refresh = () => qc.invalidateQueries({ queryKey: ['items', item.board_id] })
  const run = useMutation({
    mutationFn: (fn: () => Promise<unknown>) => fn(),
    onSuccess: () => {
      onError(null)
      void refresh()
    },
    onError: (err) => onError(errorMessage(err)),
  })
  const status = statusCol ? (item.nilai[statusCol.id] as string | undefined) : undefined
  const assigned = personCol ? ((item.nilai[personCol.id] as string[] | undefined) ?? []) : []
  const label = `item ${item.judul}`

  function saveTitle() {
    const v = judul.trim()
    if (!v || v === item.judul) {
      setJudul(item.judul)
      return
    }
    run.mutate(() => api('PATCH', `/items/${item.id}`, { judul: v }))
  }

  return (
    <li className="row-h gap-d flex items-center border-b border-line">
      {item.bisa_ubah ? (
        <input
          aria-label={`Judul ${label}`}
          className="px-d flex-1 rounded border border-transparent hover:border-line focus:border-line"
          value={judul}
          onChange={(e) => setJudul(e.target.value)}
          onBlur={saveTitle}
          onKeyDown={(e) => {
            if (e.key === 'Enter') e.currentTarget.blur()
          }}
        />
      ) : (
        <span className="px-d flex-1">{item.judul}</span>
      )}
      {statusCol && (
        <select
          aria-label={`Status ${label}`}
          className="w-36 rounded border border-line bg-surface px-1"
          value={status ?? ''}
          disabled={!item.bisa_ubah || run.isPending}
          onChange={(e) => {
            // Ambil nilai sekarang: select terkontrol kembali ke nilai lama sebelum mutasi berjalan.
            const nilai = e.target.value || null
            run.mutate(() => api('PUT', `/items/${item.id}/values/${statusCol.id}`, { nilai }))
          }}
        >
          <option value="">—</option>
          {statusCol.konfigurasi.labels?.map((l) => (
            <option key={l.id} value={l.id}>
              {l.judul}
            </option>
          ))}
        </select>
      )}
      {personCol && (
        <span className="w-40 truncate text-muted" title={personCol.judul}>
          {assigned.map((id) => names.get(id) ?? 'Pengguna').join(', ') || '—'}
        </span>
      )}
      <select
        aria-label={`Pindah grup ${label}`}
        className="w-36 rounded border border-line bg-surface px-1"
        value={item.group_id}
        disabled={!item.bisa_ubah || run.isPending}
        onChange={(e) => {
          const groupId = e.target.value
          run.mutate(() => api('POST', `/items/${item.id}/move`, { group_id: groupId }))
        }}
      >
        {groups.map((g) => (
          <option key={g.id} value={g.id}>
            {g.judul}
          </option>
        ))}
      </select>
      {item.bisa_hapus && (
        <Button
          variant="danger"
          aria-label={`Hapus ${label}`}
          disabled={run.isPending}
          onClick={() => {
            if (window.confirm(`Hapus "${item.judul}"?`)) run.mutate(() => api('DELETE', `/items/${item.id}`))
          }}
        >
          Hapus
        </Button>
      )}
    </li>
  )
}

function AddItemForm({ boardId, group }: { boardId: string; group: Group }) {
  const qc = useQueryClient()
  const [judul, setJudul] = useState('')
  const add = useMutation({
    mutationFn: () => api<Item>('POST', `/boards/${boardId}/items`, { group_id: group.id, judul }),
    onSuccess: () => {
      setJudul('')
      void qc.invalidateQueries({ queryKey: ['items', boardId] })
    },
  })

  function submit(e: FormEvent) {
    e.preventDefault()
    if (judul.trim()) add.mutate()
  }

  return (
    <form onSubmit={submit} className="gap-d flex items-center py-1">
      <input
        aria-label={`Item baru di ${group.judul}`}
        placeholder="+ Tambah item"
        className="row-h px-d flex-1 rounded border border-line"
        value={judul}
        onChange={(e) => setJudul(e.target.value)}
      />
      {add.isError && <span className="text-danger">{errorMessage(add.error)}</span>}
    </form>
  )
}

function AddGroupForm({ boardId }: { boardId: string }) {
  const qc = useQueryClient()
  const [judul, setJudul] = useState('')
  const add = useMutation({
    mutationFn: () => api<Group>('POST', `/boards/${boardId}/groups`, { judul }),
    onSuccess: () => {
      setJudul('')
      void qc.invalidateQueries({ queryKey: ['board', boardId] })
    },
  })

  function submit(e: FormEvent) {
    e.preventDefault()
    if (judul.trim()) add.mutate()
  }

  return (
    <form onSubmit={submit} className="gap-d flex items-center" aria-label="Tambah grup">
      <input
        aria-label="Judul grup baru"
        placeholder="Judul grup baru"
        className="row-h px-d rounded border border-line"
        value={judul}
        onChange={(e) => setJudul(e.target.value)}
      />
      <Button type="submit" disabled={!judul.trim() || add.isPending}>
        Tambah grup
      </Button>
      {add.isError && <span className="text-danger">{errorMessage(add.error)}</span>}
    </form>
  )
}

export function BoardPage() {
  const { boardId = '' } = useParams()
  const [rowError, setRowError] = useState<string | null>(null)
  const board = useQuery({
    queryKey: ['board', boardId],
    queryFn: () => api<BoardDetail>('GET', `/boards/${boardId}`),
    retry: false,
  })
  const items = useQuery({ queryKey: ['items', boardId], queryFn: () => fetchAllItems(boardId), enabled: board.isSuccess })
  const projectId = board.data?.project_id
  const members = useQuery({
    queryKey: ['members', projectId],
    queryFn: () => api<{ items: Member[] }>('GET', `/projects/${projectId}/members`),
    enabled: !!projectId,
  })

  if (board.isPending) return <Loading />
  if (board.isError) return <NotFoundPage />
  const b = board.data
  const statusCol = b.kolom.find((c) => c.tipe === 'status')
  const personCol = b.kolom.find((c) => c.tipe === 'person')
  const names = new Map((members.data?.items ?? []).map((m) => [m.user_id, m.nama]))
  const canCreate = b.peran_saya === 'manager' || b.peran_saya === 'member'

  return (
    <div className="gap-d flex flex-col">
      <div>
        <Link to={`/proyek/${b.project_id}`} className="text-muted hover:underline">
          Kembali ke proyek
        </Link>
        <h1 className="text-lg font-semibold">{b.nama}</h1>
      </div>
      {rowError && <Alert>{rowError}</Alert>}
      {items.isPending && <Loading />}
      {items.isError && <Alert>{errorMessage(items.error)}</Alert>}
      {items.data &&
        groupItems(b.grup, items.data).map(({ group, items: groupList }) => (
          <section
            key={group.id}
            aria-label={`Grup ${group.judul}`}
            className={`tag-${group.warna} tag-bar rounded border border-line bg-surface`}
          >
            <h2 className="px-d row-h flex items-center font-semibold">
              {group.judul} <span className="ml-2 font-normal text-muted">{groupList.length} item</span>
            </h2>
            <ul className="px-d">
              {groupList.map((it) => (
                <ItemRow
                  key={`${it.id}-${it.diubah_pada}`}
                  item={it}
                  groups={b.grup}
                  statusCol={statusCol}
                  personCol={personCol}
                  names={names}
                  onError={setRowError}
                />
              ))}
            </ul>
            {canCreate && (
              <div className="px-d">
                <AddItemForm boardId={b.id} group={group} />
              </div>
            )}
          </section>
        ))}
      {b.peran_saya === 'manager' && <AddGroupForm boardId={b.id} />}
    </div>
  )
}
