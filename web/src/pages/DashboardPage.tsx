import { useQuery } from '@tanstack/react-query'
import { useId } from 'react'
import { useSearchParams } from 'react-router'

import { api, errorMessage } from '../api/client'
import type { DashboardPanels, Project } from '../api/types'
import { Alert, Button, Loading } from '../components/ui'
import { useLiveUpdates, type LiveState } from '../dashboard/live'
import { FinancePanel, StatusPanel, TrendPanel, WorkloadPanel } from '../dashboard/Panels'
import { PROJECTS_KEY } from './ProjectsPage'

export const PANELS_KEY = ['dashboard-panels'] as const

const LIVE_TEXT: Record<LiveState, string> = {
  menyambung: 'Menyambung pembaruan langsung…',
  langsung: 'Pembaruan langsung aktif',
  terputus: 'Pembaruan langsung terputus; muat ulang halaman.',
  tidak_didukung: 'Browser tidak mendukung pembaruan langsung.',
}

/** Dasbor M6: sebaran status, beban kerja, tren penyelesaian, dan keuangan. */
export function DashboardPage() {
  const [params, setParams] = useSearchParams()
  const projectId = params.get('proyek')
  const scopeId = useId()
  const projects = useQuery({
    queryKey: [...PROJECTS_KEY, false],
    queryFn: () => api<{ items: Project[] }>('GET', '/projects?arsip=false'),
  })
  const panels = useQuery({
    queryKey: [...PANELS_KEY, projectId ?? 'semua'],
    queryFn: () =>
      api<DashboardPanels>('GET', projectId ? `/dashboard?project=${encodeURIComponent(projectId)}` : '/dashboard'),
    retry: false,
  })
  const live = useLiveUpdates(projectId, [PANELS_KEY])
  const scopeName = projectId ? (projects.data?.items.find((p) => p.id === projectId)?.nama ?? 'Proyek') : 'Semua proyek'

  return (
    <div className="gap-d flex flex-col">
      <div className="gap-d flex flex-wrap items-end justify-between">
        <div>
          <h1 className="text-(length:--heading-size) font-semibold">Dasbor</h1>
          <p className="text-muted-foreground">
            {scopeName}
            {panels.data && ` · per ${formatToday(panels.data.hari_ini)}`}
          </p>
        </div>
        <div className="gap-d flex flex-wrap items-end print:hidden">
          <label htmlFor={scopeId} className="gap-(--space-sm) flex flex-col">
            Lingkup
            <select
              id={scopeId}
              className="row-h px-d rounded border border-line bg-surface"
              value={projectId ?? ''}
              onChange={(e) =>
                setParams(
                  (p) => {
                    const next = new URLSearchParams(p)
                    if (e.target.value) next.set('proyek', e.target.value)
                    else next.delete('proyek')
                    return next
                  },
                  { replace: true },
                )
              }
            >
              <option value="">Semua proyek</option>
              {projects.data?.items.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.nama}
                </option>
              ))}
            </select>
          </label>
          <Button onClick={() => window.print()}>Cetak / simpan PDF</Button>
        </div>
      </div>
      <p role="status" className="text-muted-foreground print:hidden">
        {LIVE_TEXT[live]}
      </p>
      {panels.isPending && <Loading />}
      {panels.isError && <Alert>{errorMessage(panels.error)}</Alert>}
      {panels.data && (
        <div className="gap-d grid grid-cols-1 lg:grid-cols-2">
          <StatusPanel data={panels.data} />
          <TrendPanel data={panels.data} />
          <WorkloadPanel data={panels.data} />
          <FinancePanel data={panels.data} />
        </div>
      )}
    </div>
  )
}

function formatToday(iso: string) {
  return new Intl.DateTimeFormat('id-ID', { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'UTC' }).format(
    new Date(`${iso}T00:00:00Z`),
  )
}
