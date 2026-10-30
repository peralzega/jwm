import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router'

import { api, errorMessage } from '../api/client'
import { PRIORITIES, PROJECT_ROLE_LABEL, type Priority, type Project } from '../api/types'
import { useMe } from '../auth/session'
import { Alert, Button, Loading, Panel, SelectField, TextField } from '../components/ui'

export const PROJECTS_KEY = ['projects'] as const

function CreateProjectForm() {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [nama, setNama] = useState('')
  const [prioritas, setPrioritas] = useState<Priority>('sedang')
  const [mulai, setMulai] = useState('')
  const [selesai, setSelesai] = useState('')
  const create = useMutation({
    mutationFn: () =>
      api<Project>('POST', '/projects', { nama, prioritas, mulai: mulai || null, selesai: selesai || null }),
    onSuccess: (p) => {
      void qc.invalidateQueries({ queryKey: PROJECTS_KEY })
      navigate(`/proyek/${p.id}`)
    },
  })

  function submit(e: FormEvent) {
    e.preventDefault()
    create.mutate()
  }

  return (
    <form onSubmit={submit} className="gap-d grid grid-cols-1 items-end md:grid-cols-5" aria-label="Buat proyek">
      <TextField label="Nama proyek" value={nama} onChange={(e) => setNama(e.target.value)} required />
      <SelectField label="Prioritas" value={prioritas} onChange={(e) => setPrioritas(e.target.value as Priority)}>
        {PRIORITIES.map((p) => (
          <option key={p} value={p}>
            {p}
          </option>
        ))}
      </SelectField>
      <TextField label="Mulai" type="date" value={mulai} onChange={(e) => setMulai(e.target.value)} />
      <TextField label="Selesai" type="date" value={selesai} onChange={(e) => setSelesai(e.target.value)} />
      <Button type="submit" variant="primary" disabled={create.isPending || !nama}>
        Buat proyek
      </Button>
      {create.isError && (
        <div className="md:col-span-5">
          <Alert>{errorMessage(create.error)}</Alert>
        </div>
      )}
    </form>
  )
}

export function ProjectsPage() {
  const me = useMe()
  const [arsip, setArsip] = useState(false)
  const projects = useQuery({
    queryKey: [...PROJECTS_KEY, arsip],
    queryFn: () => api<{ items: Project[] }>('GET', `/projects?arsip=${arsip}`),
  })
  const canCreate = ['admin', 'pmo', 'project_manager'].includes(me.data?.peran ?? '')

  return (
    <div className="gap-d flex flex-col">
      <h1 className="text-lg font-semibold">Proyek</h1>
      {canCreate && (
        <Panel title="Proyek baru">
          <CreateProjectForm />
        </Panel>
      )}
      <Panel
        title={arsip ? 'Proyek diarsipkan' : 'Proyek aktif'}
        actions={<Button onClick={() => setArsip(!arsip)}>{arsip ? 'Lihat aktif' : 'Lihat arsip'}</Button>}
      >
        {projects.isPending && <Loading />}
        {projects.isError && <Alert>{errorMessage(projects.error)}</Alert>}
        {projects.data?.items.length === 0 && <p className="text-muted">Belum ada proyek.</p>}
        {projects.data && projects.data.items.length > 0 && (
          <table className="w-full text-left">
            <thead>
              <tr className="row-h border-b border-line text-muted">
                <th className="px-d font-medium">Nama</th>
                <th className="px-d font-medium">Pemilik</th>
                <th className="px-d font-medium">Prioritas</th>
                <th className="px-d font-medium">Jadwal</th>
                <th className="px-d font-medium">Peran saya</th>
              </tr>
            </thead>
            <tbody>
              {projects.data.items.map((p) => (
                <tr key={p.id} className="row-h border-b border-line">
                  <td className="px-d">
                    <Link to={`/proyek/${p.id}`} className="text-accent hover:underline">
                      {p.nama}
                    </Link>
                  </td>
                  <td className="px-d">{p.pemilik_nama}</td>
                  <td className="px-d">{p.prioritas}</td>
                  <td className="px-d">
                    {p.mulai ?? '—'} s.d. {p.selesai ?? '—'}
                  </td>
                  <td className="px-d">{p.peran_saya ? PROJECT_ROLE_LABEL[p.peran_saya] : '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Panel>
    </div>
  )
}
