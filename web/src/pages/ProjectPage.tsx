import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router'

import { api, errorMessage } from '../api/client'
import {
  PRIORITIES,
  PROJECT_ROLE_LABEL,
  ROLE_LABEL,
  type Board,
  type BoardDetail,
  type Member,
  type Priority,
  type Project,
  type ProjectRole,
  type UserSummary,
} from '../api/types'
import { Alert, Button, Loading, Panel, SelectField, TextField } from '../components/ui'
import { BudgetPanel } from '../dashboard/BudgetPanel'
import { DASHBOARD_KEY, PORTFOLIOS_KEY, usePortfolios } from '../portfolio/queries'
import { NotFoundPage } from './NotFoundPage'
import { PROJECTS_KEY } from './ProjectsPage'

const PROJECT_ROLES = Object.keys(PROJECT_ROLE_LABEL) as ProjectRole[]

function ProjectSettings({ project }: { project: Project }) {
  const qc = useQueryClient()
  const [nama, setNama] = useState(project.nama)
  const [prioritas, setPrioritas] = useState<Priority>(project.prioritas)
  const [mulai, setMulai] = useState(project.mulai ?? '')
  const [selesai, setSelesai] = useState(project.selesai ?? '')
  const [portfolioId, setPortfolioId] = useState(project.portfolio_id ?? '')
  const portfolios = usePortfolios()
  const refresh = (p: Project) => {
    qc.setQueryData(['project', p.id], p)
    void qc.invalidateQueries({ queryKey: PROJECTS_KEY })
    void qc.invalidateQueries({ queryKey: DASHBOARD_KEY })
    void qc.invalidateQueries({ queryKey: PORTFOLIOS_KEY })
  }
  const save = useMutation({
    mutationFn: () =>
      api<Project>('PATCH', `/projects/${project.id}`, {
        nama,
        prioritas,
        mulai: mulai || null,
        selesai: selesai || null,
        portfolio_id: portfolioId || null,
      }),
    onSuccess: refresh,
  })
  const archive = useMutation({
    mutationFn: (arsip: boolean) => api<Project>('POST', `/projects/${project.id}/${arsip ? 'archive' : 'unarchive'}`),
    onSuccess: refresh,
  })
  const archived = project.diarsipkan_pada !== null

  function submit(e: FormEvent) {
    e.preventDefault()
    save.mutate()
  }

  return (
    <Panel
      title="Pengaturan proyek"
      actions={
        <Button variant={archived ? 'secondary' : 'danger'} onClick={() => archive.mutate(!archived)}>
          {archived ? 'Pulihkan dari arsip' : 'Arsipkan'}
        </Button>
      }
    >
      <form onSubmit={submit} className="gap-d grid grid-cols-1 items-end md:grid-cols-6" aria-label="Ubah proyek">
        <TextField label="Nama proyek" value={nama} onChange={(e) => setNama(e.target.value)} disabled={archived} />
        <SelectField
          label="Prioritas"
          value={prioritas}
          onChange={(e) => setPrioritas(e.target.value as Priority)}
          disabled={archived}
        >
          {PRIORITIES.map((p) => (
            <option key={p} value={p}>
              {p}
            </option>
          ))}
        </SelectField>
        <TextField label="Mulai" type="date" value={mulai} onChange={(e) => setMulai(e.target.value)} disabled={archived} />
        <TextField
          label="Selesai"
          type="date"
          value={selesai}
          onChange={(e) => setSelesai(e.target.value)}
          disabled={archived}
        />
        <SelectField
          label="Portofolio"
          value={portfolioId}
          onChange={(e) => setPortfolioId(e.target.value)}
          disabled={archived}
        >
          <option value="">Tanpa portofolio</option>
          {portfolios.data?.map((pf) => (
            <option key={pf.id} value={pf.id}>
              {pf.nama}
            </option>
          ))}
        </SelectField>
        <Button type="submit" variant="primary" disabled={archived || save.isPending}>
          Simpan
        </Button>
      </form>
      {(save.isError || archive.isError) && <Alert>{errorMessage(save.error ?? archive.error)}</Alert>}
      {save.isSuccess && <Alert kind="ok">Perubahan disimpan.</Alert>}
    </Panel>
  )
}

function AddMemberForm({ projectId }: { projectId: string }) {
  const qc = useQueryClient()
  const [q, setQ] = useState('')
  const [userId, setUserId] = useState('')
  const [peran, setPeran] = useState<ProjectRole>('member')
  const search = useQuery({
    queryKey: ['user-search', q],
    queryFn: () => api<{ items: UserSummary[] }>('GET', `/users?q=${encodeURIComponent(q)}`),
    enabled: q.trim().length >= 2,
  })
  const add = useMutation({
    mutationFn: () => api<{ items: Member[] }>('POST', `/projects/${projectId}/members`, { user_id: userId, peran_proyek: peran }),
    onSuccess: (r) => {
      qc.setQueryData(['members', projectId], r)
      setQ('')
      setUserId('')
    },
  })

  function submit(e: FormEvent) {
    e.preventDefault()
    add.mutate()
  }

  return (
    <form onSubmit={submit} className="gap-d grid grid-cols-1 items-end md:grid-cols-4" aria-label="Tambah anggota">
      <TextField label="Cari pengguna" value={q} onChange={(e) => setQ(e.target.value)} hint="Minimal 2 huruf" />
      <SelectField label="Pengguna" value={userId} onChange={(e) => setUserId(e.target.value)}>
        <option value="">— pilih —</option>
        {search.data?.items.map((u) => (
          <option key={u.id} value={u.id}>
            {u.nama} ({u.email})
          </option>
        ))}
      </SelectField>
      <SelectField label="Peran proyek" value={peran} onChange={(e) => setPeran(e.target.value as ProjectRole)}>
        {PROJECT_ROLES.map((r) => (
          <option key={r} value={r}>
            {PROJECT_ROLE_LABEL[r]}
          </option>
        ))}
      </SelectField>
      <Button type="submit" variant="primary" disabled={!userId || add.isPending}>
        Tambah anggota
      </Button>
      {add.isError && (
        <div className="md:col-span-4">
          <Alert>{errorMessage(add.error)}</Alert>
        </div>
      )}
    </form>
  )
}

function MembersPanel({ project, canManage }: { project: Project; canManage: boolean }) {
  const qc = useQueryClient()
  const key = ['members', project.id]
  const members = useQuery({
    queryKey: key,
    queryFn: () => api<{ items: Member[] }>('GET', `/projects/${project.id}/members`),
  })
  const change = useMutation({
    mutationFn: async ({ userId, peran }: { userId: string; peran: ProjectRole | null }): Promise<void> => {
      if (peran) await api('PATCH', `/projects/${project.id}/members/${userId}`, { peran_proyek: peran })
      else await api('DELETE', `/projects/${project.id}/members/${userId}`)
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: key }),
  })
  const archived = project.diarsipkan_pada !== null

  return (
    <Panel title="Anggota">
      {canManage && !archived && <AddMemberForm projectId={project.id} />}
      {change.isError && <Alert>{errorMessage(change.error)}</Alert>}
      {members.isPending && <Loading />}
      {members.data && (
        <table className="mt-(--density-gap) w-full text-left">
          <thead>
            <tr className="row-h border-b border-line text-muted-foreground">
              <th className="px-d font-medium">Nama</th>
              <th className="px-d font-medium">Peran global</th>
              <th className="px-d font-medium">Peran proyek</th>
              <th className="px-d font-medium">Hak efektif</th>
              {canManage && <th className="px-d font-medium">Tindakan</th>}
            </tr>
          </thead>
          <tbody>
            {members.data.items.map((m) => (
              <tr key={m.user_id} className="row-h border-b border-line">
                <td className="px-d">
                  {m.nama} {m.user_id === project.pemilik_id && <span className="text-muted-foreground">(pemilik)</span>}
                </td>
                <td className="px-d">{ROLE_LABEL[m.peran_global]}</td>
                <td className="px-d">
                  {canManage && !archived ? (
                    <select
                      aria-label={`Peran proyek ${m.nama}`}
                      className="rounded border border-line bg-surface px-(--space-sm)"
                      value={m.peran_proyek}
                      onChange={(e) => change.mutate({ userId: m.user_id, peran: e.target.value as ProjectRole })}
                    >
                      {PROJECT_ROLES.map((r) => (
                        <option key={r} value={r}>
                          {PROJECT_ROLE_LABEL[r]}
                        </option>
                      ))}
                    </select>
                  ) : (
                    PROJECT_ROLE_LABEL[m.peran_proyek]
                  )}
                </td>
                <td className="px-d">{m.peran_efektif ? PROJECT_ROLE_LABEL[m.peran_efektif] : 'Tidak ada'}</td>
                {canManage && (
                  <td className="px-d">
                    <Button
                      variant="danger"
                      disabled={archived}
                      onClick={() => {
                        if (window.confirm(`Keluarkan ${m.nama} dari proyek?`)) change.mutate({ userId: m.user_id, peran: null })
                      }}
                    >
                      Keluarkan
                    </Button>
                  </td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </Panel>
  )
}

function BoardsPanel({ project, canManage }: { project: Project; canManage: boolean }) {
  const qc = useQueryClient()
  const key = ['boards', project.id]
  const [nama, setNama] = useState('')
  const boards = useQuery({
    queryKey: key,
    queryFn: () => api<{ items: Board[] }>('GET', `/projects/${project.id}/boards`),
  })
  const create = useMutation({
    mutationFn: () => api<BoardDetail>('POST', `/projects/${project.id}/boards`, { nama }),
    onSuccess: () => {
      setNama('')
      void qc.invalidateQueries({ queryKey: key })
    },
  })

  function submit(e: FormEvent) {
    e.preventDefault()
    create.mutate()
  }

  return (
    <Panel title="Board">
      {boards.isPending && <Loading />}
      {boards.data?.items.length === 0 && <p className="text-muted-foreground">Belum ada board.</p>}
      <ul className="flex flex-col">
        {boards.data?.items.map((b) => (
          <li key={b.id} className="row-h flex items-center border-b border-line">
            <Link to={`/board/${b.id}`} className="text-primary hover:underline">
              {b.nama}
            </Link>
          </li>
        ))}
      </ul>
      {canManage && project.diarsipkan_pada === null && (
        <form onSubmit={submit} className="gap-d mt-(--density-gap) flex items-end" aria-label="Buat board">
          <TextField label="Nama board baru" value={nama} onChange={(e) => setNama(e.target.value)} />
          <Button type="submit" variant="primary" disabled={!nama || create.isPending}>
            Buat board
          </Button>
        </form>
      )}
      {create.isError && <Alert>{errorMessage(create.error)}</Alert>}
    </Panel>
  )
}

export function ProjectPage() {
  const { projectId = '' } = useParams()
  const project = useQuery({
    queryKey: ['project', projectId],
    queryFn: () => api<Project>('GET', `/projects/${projectId}`),
    retry: false,
  })

  if (project.isPending) return <Loading />
  if (project.isError) return <NotFoundPage />
  const p = project.data
  const canManage = p.peran_saya === 'manager'

  return (
    <div className="gap-d flex flex-col">
      <div>
        <Link to="/" className="text-muted-foreground hover:underline">
          Proyek
        </Link>
        <h1 className="text-(length:--heading-size) font-semibold">
          {p.nama} {p.diarsipkan_pada && <span className="text-muted-foreground">(diarsipkan)</span>}
        </h1>
        <p className="text-muted-foreground">
          Pemilik {p.pemilik_nama} · prioritas {p.prioritas}
          {p.portfolio_nama && ` · portofolio ${p.portfolio_nama}`} · peran Anda{' '}
          {p.peran_saya ? PROJECT_ROLE_LABEL[p.peran_saya] : '—'}
        </p>
      </div>
      {canManage && <ProjectSettings key={p.diubah_pada} project={p} />}
      <BoardsPanel project={p} canManage={canManage} />
      <BudgetPanel projectId={p.id} />
      <MembersPanel project={p} canManage={canManage} />
    </div>
  )
}
