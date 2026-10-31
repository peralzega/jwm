import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'
import { useSearchParams } from 'react-router'

import { api, errorMessage } from '../api/client'
import { HEALTH_STYLE, PRIORITIES, type Health, type PortfolioDashboard, type Priority } from '../api/types'
import { useMe } from '../auth/session'
import { PRIORITY_STYLE } from '../board/format'
import { Alert, Loading } from '../components/ui'
import { PortfoliosPanel } from '../portfolio/PortfoliosPanel'
import { ProjectRow } from '../portfolio/ProjectRow'
import { DASHBOARD_KEY, usePortfolios } from '../portfolio/queries'

const SELECT = 'row-h px-d rounded border border-line bg-surface'

/**
 * Dasbor portofolio (M5): semua proyek yang dapat dilihat pengguna dengan
 * progres, prioritas, dan health. Filter disimpan di URL; penyaringan di klien
 * karena jumlah proyek kecil (< 100) dan pilihan filter perlu daftar lengkap.
 */
export function PortfolioPage() {
  const me = useMe()
  const [params, setParams] = useSearchParams()
  const dash = useQuery({
    queryKey: DASHBOARD_KEY,
    queryFn: () => api<PortfolioDashboard>('GET', '/portfolio'),
  })
  const portfolios = usePortfolios()
  const canManage = me.data?.peran === 'admin' || me.data?.peran === 'pmo'

  const f = {
    prioritas: params.get('prioritas') ?? '',
    pemilik: params.get('pemilik') ?? '',
    status: params.get('status') ?? '',
    portofolio: params.get('portofolio') ?? '',
  }
  function setFilter(key: keyof typeof f, value: string) {
    setParams(
      (p) => {
        const next = new URLSearchParams(p)
        if (value) next.set(key, value)
        else next.delete(key)
        return next
      },
      { replace: true },
    )
  }

  const owners = useMemo(() => {
    const m = new Map<string, string>()
    for (const p of dash.data?.items ?? []) m.set(p.pemilik_id, p.pemilik_nama)
    return [...m].sort((a, b) => a[1].localeCompare(b[1], 'id'))
  }, [dash.data])

  const rows = (dash.data?.items ?? []).filter(
    (p) =>
      (!f.prioritas || p.prioritas === f.prioritas) &&
      (!f.pemilik || p.pemilik_id === f.pemilik) &&
      (!f.status || p.health === f.status) &&
      (!f.portofolio || (f.portofolio === 'tanpa' ? p.portfolio_id === null : p.portfolio_id === f.portofolio)),
  )
  const counts = { hijau: 0, kuning: 0, merah: 0 }
  for (const p of rows) counts[p.health]++

  return (
    <div className="gap-d flex flex-col">
      <h1 className="text-(length:--heading-size) font-semibold">Portofolio</h1>
      <div role="group" aria-label="Filter" className="gap-d flex flex-wrap items-end">
        <label className="gap-(--space-sm) flex flex-col">
          Prioritas
          <select className={SELECT} value={f.prioritas} onChange={(e) => setFilter('prioritas', e.target.value)}>
            <option value="">Semua</option>
            {PRIORITIES.map((p: Priority) => (
              <option key={p} value={p}>
                {PRIORITY_STYLE[p].judul}
              </option>
            ))}
          </select>
        </label>
        <label className="gap-(--space-sm) flex flex-col">
          Pemilik
          <select className={SELECT} value={f.pemilik} onChange={(e) => setFilter('pemilik', e.target.value)}>
            <option value="">Semua</option>
            {owners.map(([id, nama]) => (
              <option key={id} value={id}>
                {nama}
              </option>
            ))}
          </select>
        </label>
        <label className="gap-(--space-sm) flex flex-col">
          Status
          <select className={SELECT} value={f.status} onChange={(e) => setFilter('status', e.target.value)}>
            <option value="">Semua</option>
            {(Object.keys(HEALTH_STYLE) as Health[]).map((h) => (
              <option key={h} value={h}>
                {HEALTH_STYLE[h].judul}
              </option>
            ))}
          </select>
        </label>
        <label className="gap-(--space-sm) flex flex-col">
          Portofolio
          <select className={SELECT} value={f.portofolio} onChange={(e) => setFilter('portofolio', e.target.value)}>
            <option value="">Semua</option>
            <option value="tanpa">Tanpa portofolio</option>
            {portfolios.data?.map((pf) => (
              <option key={pf.id} value={pf.id}>
                {pf.nama}
              </option>
            ))}
          </select>
        </label>
        {dash.data && (
          <p role="status" className="text-muted-foreground">
            {rows.length} proyek · {counts.merah} Merah · {counts.kuning} Kuning · {counts.hijau} Hijau
          </p>
        )}
      </div>

      {dash.isPending && <Loading />}
      {dash.isError && <Alert>{errorMessage(dash.error)}</Alert>}
      {dash.data && rows.length === 0 && (
        <p className="text-muted-foreground">Tidak ada proyek yang cocok dengan filter.</p>
      )}
      {dash.data && rows.length > 0 && (
        <div className="overflow-x-auto rounded border border-line bg-surface">
          <table className="w-full text-left">
            <caption className="sr-only">Proyek, progres, dan health</caption>
            <thead>
              <tr className="row-h border-b border-line text-muted-foreground">
                <th className="px-d font-medium">Proyek</th>
                <th className="px-d font-medium">Portofolio</th>
                <th className="px-d font-medium">Pemilik</th>
                <th className="px-d font-medium">Prioritas</th>
                <th className="px-d font-medium">Progres</th>
                <th className="px-d font-medium">Health</th>
                <th className="px-d font-medium">Item berisiko</th>
                <th className="px-d font-medium">
                  <span className="sr-only">Aksi</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {rows.map((p) => (
                <ProjectRow key={p.id} p={p} canOverride={canManage} />
              ))}
            </tbody>
          </table>
        </div>
      )}
      {dash.data && (
        <p className="text-muted-foreground">
          Merah: ada item jalur kritis terlambat atau lebih dari {dash.data.ambang.merah_persen}% item berisiko.
          Kuning: {dash.data.ambang.kuning_persen}–{dash.data.ambang.merah_persen}% item berisiko atau progres
          tertinggal lebih dari {dash.data.ambang.tertinggal_poin} poin dari rencana. Dihitung per{' '}
          {dash.data.hari_ini}.
        </p>
      )}
      {canManage && <PortfoliosPanel />}
    </div>
  )
}
