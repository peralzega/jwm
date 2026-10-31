import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'

import { api, errorMessage } from '../api/client'
import type { Portfolio } from '../api/types'
import { Alert, Button, Panel } from '../components/ui'
import { DASHBOARD_KEY, PORTFOLIOS_KEY, usePortfolios } from './queries'

function useRefresh() {
  const qc = useQueryClient()
  return () => {
    void qc.invalidateQueries({ queryKey: PORTFOLIOS_KEY })
    void qc.invalidateQueries({ queryKey: DASHBOARD_KEY })
  }
}

function PortfolioRow({ pf, onError }: { pf: Portfolio; onError: (m: string | null) => void }) {
  const refresh = useRefresh()
  const [nama, setNama] = useState(pf.nama)
  const rename = useMutation({
    mutationFn: () => api<Portfolio>('PATCH', `/portfolios/${pf.id}`, { nama }),
    onSuccess: () => {
      onError(null)
      refresh()
    },
    onError: (e) => onError(errorMessage(e)),
  })
  const remove = useMutation({
    mutationFn: () => api<undefined>('DELETE', `/portfolios/${pf.id}`),
    onSuccess: refresh,
    onError: (e) => onError(errorMessage(e)),
  })
  return (
    <li className="row-h gap-d flex items-center">
      <input
        aria-label={`Nama portofolio ${pf.nama}`}
        className="row-h px-d min-w-0 flex-1 rounded border border-line bg-surface"
        maxLength={200}
        value={nama}
        onChange={(e) => setNama(e.target.value)}
      />
      <span className="text-muted-foreground tabular-nums">{pf.jumlah_proyek} proyek</span>
      <Button disabled={!nama.trim() || nama === pf.nama || rename.isPending} onClick={() => rename.mutate()}>
        Ganti nama
      </Button>
      <Button
        variant="danger"
        aria-label={`Hapus portofolio ${pf.nama}`}
        disabled={remove.isPending}
        onClick={() => {
          if (window.confirm(`Hapus portofolio "${pf.nama}"? Proyeknya tidak ikut terhapus.`)) remove.mutate()
        }}
      >
        Hapus
      </Button>
    </li>
  )
}

/** Kelola portofolio (Admin dan PMO). */
export function PortfoliosPanel() {
  const portfolios = usePortfolios()
  const refresh = useRefresh()
  const [nama, setNama] = useState('')
  const [error, setError] = useState<string | null>(null)
  const create = useMutation({
    mutationFn: () => api<Portfolio>('POST', '/portfolios', { nama }),
    onSuccess: () => {
      setNama('')
      setError(null)
      refresh()
    },
    onError: (e) => setError(errorMessage(e)),
  })

  function submit(e: FormEvent) {
    e.preventDefault()
    if (nama.trim()) create.mutate()
  }

  return (
    <Panel title="Kelola portofolio">
      <div className="gap-d flex flex-col">
        <form onSubmit={submit} aria-label="Tambah portofolio" className="gap-d flex items-center">
          <input
            aria-label="Nama portofolio baru"
            placeholder="Nama portofolio baru"
            className="row-h px-d min-w-0 flex-1 rounded border border-line bg-surface"
            maxLength={200}
            value={nama}
            onChange={(e) => setNama(e.target.value)}
          />
          <Button type="submit" variant="primary" disabled={!nama.trim() || create.isPending}>
            Tambah
          </Button>
        </form>
        {error && <Alert>{error}</Alert>}
        <ul className="flex flex-col gap-(--space-sm)">
          {portfolios.data?.map((pf) => (
            <PortfolioRow key={`${pf.id}:${pf.nama}`} pf={pf} onError={setError} />
          ))}
        </ul>
      </div>
    </Panel>
  )
}
