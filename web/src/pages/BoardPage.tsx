import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { Link, useParams, useSearchParams } from 'react-router'

import { api, errorMessage } from '../api/client'
import type { BoardDetail, Group, Item, Member, ShiftedItem } from '../api/types'
import { BoardTable } from '../board/BoardTable'
import { DependencyProvider } from '../board/deps/DependencyContext'
import { ShiftNotice } from '../board/deps/ShiftNotice'
import { parseScale, SCALES } from '../board/gantt/scale'
import { statusColumns } from '../board/kanban/columns'
import { LazyGantt } from '../board/LazyGantt'
import { LazyKanban } from '../board/LazyKanban'
import { isBoardView, parseView, ViewTabsList } from '../board/ViewTabs'
import { Tabs, TabsContent } from '../components/ui/tabs'
import { useBoardItems, useItemMutations } from '../board/useBoardItems'
import { Alert, Button, Loading } from '../components/ui'
import { NotFoundPage } from './NotFoundPage'

const NO_MEMBERS: Member[] = []
const NO_ITEMS: Item[] = []

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
  const [params, setParams] = useSearchParams()
  const [error, setError] = useState<string | null>(null)
  const [shifted, setShifted] = useState<ShiftedItem[]>([])
  const board = useQuery({
    queryKey: ['board', boardId],
    queryFn: () => api<BoardDetail>('GET', `/boards/${boardId}`),
    retry: false,
  })
  const items = useBoardItems(boardId, board.isSuccess)
  const mutations = useItemMutations(boardId, setError, setShifted)
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
  const view = parseView(params.get('tampilan'))
  const statusCols = statusColumns(b.kolom)
  const statusCol = statusCols.find((c) => c.id === params.get('status')) ?? statusCols[0]
  const timelineCols = b.kolom.filter((c) => c.tipe === 'timeline').sort((x, y) => x.urutan - y.urutan)
  const timelineCol = timelineCols.find((c) => c.id === params.get('timeline')) ?? timelineCols[0]
  const scale = parseScale(params.get('skala'))
  const hasDependencyCol = b.kolom.some((c) => c.tipe === 'dependency')

  function setParam(key: string, value: string | null) {
    setParams(
      (p) => {
        const next = new URLSearchParams(p)
        if (value === null) next.delete(key)
        else next.set(key, value)
        return next
      },
      { replace: true },
    )
  }

  return (
    <div className="gap-d flex flex-col">
      <div className="gap-d flex flex-wrap items-end justify-between">
        <div>
          <Link to={`/proyek/${b.project_id}`} className="text-muted-foreground hover:underline">
            Kembali ke proyek
          </Link>
          <h1 className="text-(length:--heading-size) font-semibold">{b.nama}</h1>
        </div>
        {b.peran_saya === 'manager' && <AddGroupForm boardId={b.id} />}
      </div>
      <Tabs
        value={view}
        onValueChange={(v) => {
          if (isBoardView(v)) setParam('tampilan', v === 'tabel' ? null : v)
        }}
      >
      <div className="gap-d flex flex-wrap items-end justify-between">
        <ViewTabsList />
        {view === 'kanban' && statusCols.length > 1 && statusCol && (
          <label className="gap-d flex items-center">
            Kelompokkan menurut
            <select
              className="row-h px-d rounded border border-line bg-surface"
              value={statusCol.id}
              onChange={(e) => setParam('status', e.target.value)}
            >
              {statusCols.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.judul}
                </option>
              ))}
            </select>
          </label>
        )}
        {view === 'gantt' && (
          <div className="gap-d flex flex-wrap items-center">
            {timelineCols.length > 1 && timelineCol && (
              <label className="gap-d flex items-center">
                Kolom timeline
                <select
                  className="row-h px-d rounded border border-line bg-surface"
                  value={timelineCol.id}
                  onChange={(e) => setParam('timeline', e.target.value)}
                >
                  {timelineCols.map((c) => (
                    <option key={c.id} value={c.id}>
                      {c.judul}
                    </option>
                  ))}
                </select>
              </label>
            )}
            <div role="group" aria-label="Skala waktu" className="flex">
              {SCALES.map((s) => (
                <Button
                  key={s.id}
                  variant={s.id === scale ? 'primary' : 'secondary'}
                  aria-pressed={s.id === scale}
                  onClick={() => setParam('skala', s.id === 'minggu' ? null : s.id)}
                >
                  {s.label}
                </Button>
              ))}
            </div>
          </div>
        )}
      </div>
      <div aria-live="polite">
        {error && <Alert>{error}</Alert>}
        {shifted.length > 0 && <ShiftNotice items={shifted} onDismiss={() => setShifted([])} />}
      </div>
      {items.isPending && (
        <p role="status" className="px-d py-d text-muted-foreground">
          Memuat item… {items.loaded > 0 && `${items.loaded.toLocaleString('id-ID')} dimuat`}
        </p>
      )}
      {items.isError && <Alert>{errorMessage(items.error)}</Alert>}
      <DependencyProvider boardId={b.id} items={items.data ?? NO_ITEMS} enabled={!!items.data && (view === 'gantt' || hasDependencyCol)}>
      <TabsContent value="kanban">
        {items.data && !statusCol && (
          <p className="px-d py-d text-muted-foreground">Board ini belum memiliki kolom status untuk Kanban.</p>
        )}
        {items.data && statusCol && (
          <LazyKanban
            board={b}
            statusCol={statusCol}
            items={items.data}
            members={members.data?.items ?? NO_MEMBERS}
            mutations={mutations}
          />
        )}
      </TabsContent>
      <TabsContent value="gantt">
        {items.data && !timelineCol && (
          <p className="px-d py-d text-muted-foreground">Board ini belum memiliki kolom timeline untuk Gantt.</p>
        )}
        {items.data && timelineCol && (
          <LazyGantt
            board={b}
            items={items.data}
            timelineCol={timelineCol}
            scale={scale}
            mutations={mutations}
            onError={setError}
          />
        )}
      </TabsContent>
      <TabsContent value="tabel">
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
      </TabsContent>
      </DependencyProvider>
      </Tabs>
    </div>
  )
}
