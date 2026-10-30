import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router'

import { api, errorMessage } from '../api/client'
import type { BoardDetail, Group, Member } from '../api/types'
import { BoardTable } from '../board/BoardTable'
import { useBoardItems, useItemMutations } from '../board/useBoardItems'
import { Alert, Button, Loading } from '../components/ui'
import { NotFoundPage } from './NotFoundPage'

const NO_MEMBERS: Member[] = []

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
        maxLength={100}
        className="row-h px-d rounded border border-line bg-surface"
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
  const [error, setError] = useState<string | null>(null)
  const board = useQuery({
    queryKey: ['board', boardId],
    queryFn: () => api<BoardDetail>('GET', `/boards/${boardId}`),
    retry: false,
  })
  const items = useBoardItems(boardId, board.isSuccess)
  const mutations = useItemMutations(boardId, setError)
  const projectId = board.data?.project_id
  const members = useQuery({
    queryKey: ['members', projectId],
    queryFn: () => api<{ items: Member[] }>('GET', `/projects/${projectId}/members`),
    enabled: !!projectId,
  })

  if (board.isPending) return <Loading />
  if (board.isError) return <NotFoundPage />
  const b = board.data
  const canCreate = b.peran_saya === 'manager' || b.peran_saya === 'member'

  return (
    <div className="gap-d flex flex-col">
      <div className="gap-d flex flex-wrap items-end justify-between">
        <div>
          <Link to={`/proyek/${b.project_id}`} className="text-muted hover:underline">
            Kembali ke proyek
          </Link>
          <h1 className="text-lg font-semibold">{b.nama}</h1>
        </div>
        {b.peran_saya === 'manager' && <AddGroupForm boardId={b.id} />}
      </div>
      <div aria-live="polite">{error && <Alert>{error}</Alert>}</div>
      {items.isPending && (
        <p role="status" className="px-d py-d text-muted">
          Memuat item… {items.loaded > 0 && `${items.loaded.toLocaleString('id-ID')} dimuat`}
        </p>
      )}
      {items.isError && <Alert>{errorMessage(items.error)}</Alert>}
      {items.data && (
        <BoardTable
          board={b}
          items={items.data}
          members={members.data?.items ?? NO_MEMBERS}
          canCreate={canCreate}
          mutations={mutations}
          onError={setError}
        />
      )}
    </div>
  )
}
