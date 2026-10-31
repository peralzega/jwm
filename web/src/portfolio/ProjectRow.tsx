import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useId, useState, type FormEvent } from 'react'
import { Link } from 'react-router'

import { api, errorMessage } from '../api/client'
import { HEALTH_STYLE, RISK_RULES, type Health, type ProjectHealth, type RiskItem } from '../api/types'
import { formatDate, PRIORITY_STYLE } from '../board/format'
import { Alert, Button } from '../components/ui'
import { DASHBOARD_KEY } from './queries'

const COLS = 8

export function HealthBadge({ health, manual }: { health: Health; manual?: string }) {
  const s = HEALTH_STYLE[health]
  return (
    <span className="gap-(--space-sm) inline-flex items-center">
      <span className={`tag-${s.warna} tag-fill rounded px-(--space-sm) font-medium`}>{s.judul}</span>
      {manual !== undefined && (
        <span className="text-muted-foreground" title={manual}>
          manual
        </span>
      )}
    </span>
  )
}

function ProgressCell({ p }: { p: ProjectHealth }) {
  if (p.progres === null) return <span className="text-muted-foreground">—</span>
  return (
    <span className="gap-d flex items-center">
      <span className="h-(--bar-h) w-(--progress-bar-w) overflow-hidden rounded-full bg-canvas" aria-hidden="true">
        <span className="block h-full rounded-full bg-primary" style={{ width: `${Math.min(100, p.progres)}%` }} />
      </span>
      <span className="tabular-nums">
        {p.progres.toLocaleString('id-ID')}%
        {p.progres_rencana !== null && (
          <span className="text-muted-foreground"> (rencana {p.progres_rencana.toLocaleString('id-ID')}%)</span>
        )}
      </span>
    </span>
  )
}

function RiskItems({ projectId }: { projectId: string }) {
  const risks = useQuery({
    queryKey: [...DASHBOARD_KEY, 'risks', projectId],
    queryFn: () => api<{ items: RiskItem[] }>('GET', `/portfolio/projects/${projectId}/risks`).then((r) => r.items),
  })
  if (risks.isPending) return <p className="text-muted-foreground">Memuat item berisiko…</p>
  if (risks.isError) return <Alert>{errorMessage(risks.error)}</Alert>
  if (risks.data.length === 0) return <p className="text-muted-foreground">Tidak ada item berisiko.</p>
  return (
    <ul aria-label="Item berisiko" className="flex flex-col gap-(--space-sm)">
      {risks.data.map((it) => (
        <li key={it.item_id}>
          <Link to={`/board/${it.board_id}`} className="font-medium hover:underline">
            {it.judul}
          </Link>
          <span className="text-muted-foreground">
            {' '}
            · {it.board_nama}
            {it.kritis && ' · jalur kritis'}
          </span>
          <ul className="pl-(--list-indent) text-muted-foreground">
            {it.alasan.map((a) => (
              <li key={a.kode}>{a.pesan}</li>
            ))}
          </ul>
        </li>
      ))}
    </ul>
  )
}

function OverrideForm({ p, onDone }: { p: ProjectHealth; onDone: () => void }) {
  const qc = useQueryClient()
  const [status, setStatus] = useState<Health>(p.status_manual?.status ?? p.health_otomatis)
  const [alasan, setAlasan] = useState(p.status_manual?.alasan ?? '')
  const ids = { status: useId(), alasan: useId() }
  const done = () => {
    void qc.invalidateQueries({ queryKey: DASHBOARD_KEY })
    onDone()
  }
  const save = useMutation({
    mutationFn: () => api<undefined>('PUT', `/projects/${p.id}/status-manual`, { status, alasan }),
    onSuccess: done,
  })
  const clear = useMutation({
    mutationFn: () => api<undefined>('DELETE', `/projects/${p.id}/status-manual`),
    onSuccess: done,
  })

  function submit(e: FormEvent) {
    e.preventDefault()
    save.mutate()
  }

  return (
    <form onSubmit={submit} aria-label={`Timpa status ${p.nama}`} className="gap-d flex flex-wrap items-end">
      <label htmlFor={ids.status} className="flex flex-col gap-(--space-sm)">
        Status
        <select
          id={ids.status}
          className="row-h px-d rounded border border-line bg-surface"
          value={status}
          onChange={(e) => setStatus(e.target.value as Health)}
        >
          {Object.entries(HEALTH_STYLE).map(([k, s]) => (
            <option key={k} value={k}>
              {s.judul}
            </option>
          ))}
        </select>
      </label>
      <label htmlFor={ids.alasan} className="flex min-w-0 flex-1 flex-col gap-(--space-sm)">
        Alasan (wajib)
        <input
          id={ids.alasan}
          className="row-h px-d rounded border border-line bg-surface"
          maxLength={500}
          value={alasan}
          onChange={(e) => setAlasan(e.target.value)}
        />
      </label>
      <Button type="submit" variant="primary" disabled={!alasan.trim() || save.isPending}>
        Simpan status
      </Button>
      {p.status_manual && (
        <Button onClick={() => clear.mutate()} disabled={clear.isPending}>
          Kembali ke otomatis
        </Button>
      )}
      <Button onClick={onDone}>Batal</Button>
      {(save.isError || clear.isError) && (
        <div className="w-full">
          <Alert>{errorMessage(save.error ?? clear.error)}</Alert>
        </div>
      )}
    </form>
  )
}

/** Satu proyek di dasbor: baris ringkas, rincian risiko, dan form timpa status. */
export function ProjectRow({ p, canOverride }: { p: ProjectHealth; canOverride: boolean }) {
  const [open, setOpen] = useState(false)
  const [overriding, setOverriding] = useState(false)
  const detailId = useId()
  const prio = PRIORITY_STYLE[p.prioritas]

  return (
    <>
      <tr className="row-h border-b border-line">
        <td className="px-d">
          <Link to={`/proyek/${p.id}`} className="font-medium hover:underline">
            {p.nama}
          </Link>
          {p.selesai && <span className="text-muted-foreground"> · selesai {formatDate(p.selesai, true)}</span>}
        </td>
        <td className="px-d">{p.portfolio_nama ?? <span className="text-muted-foreground">—</span>}</td>
        <td className="px-d">{p.pemilik_nama}</td>
        <td className="px-d">
          <span className={`tag-${prio.warna} tag-fill rounded px-(--space-sm)`}>{prio.judul}</span>
        </td>
        <td className="px-d">
          <ProgressCell p={p} />
        </td>
        <td className="px-d">
          <HealthBadge
            health={p.health}
            manual={p.status_manual ? `Ditimpa ${p.status_manual.oleh_nama}: ${p.status_manual.alasan}` : undefined}
          />
        </td>
        <td className="px-d">
          <Button
            aria-expanded={open}
            aria-controls={detailId}
            aria-label={`Rincian ${p.nama}: ${p.item_berisiko} dari ${p.jumlah_item} item berisiko`}
            onClick={() => setOpen((o) => !o)}
          >
            {p.item_berisiko} / {p.jumlah_item}
          </Button>
        </td>
        <td className="px-d">
          {canOverride && (
            <Button aria-label={`Timpa status ${p.nama}`} onClick={() => setOverriding((o) => !o)}>
              Timpa
            </Button>
          )}
        </td>
      </tr>
      {overriding && (
        <tr className="border-b border-line bg-canvas">
          <td colSpan={COLS} className="px-d py-d">
            <OverrideForm p={p} onDone={() => setOverriding(false)} />
          </td>
        </tr>
      )}
      {open && (
        <tr id={detailId} className="border-b border-line bg-canvas">
          <td colSpan={COLS} className="px-d py-d">
            <div className="gap-d grid grid-cols-1 md:grid-cols-3">
              <section aria-label={`Health ${p.nama}`} className="flex flex-col gap-(--space-sm)">
                <h3 className="font-semibold">
                  Health otomatis: {HEALTH_STYLE[p.health_otomatis].judul}
                </h3>
                <ul className="text-muted-foreground">
                  {p.health_alasan.map((a) => (
                    <li key={a}>{a}</li>
                  ))}
                </ul>
                {p.status_manual && (
                  <p>
                    Ditimpa menjadi {HEALTH_STYLE[p.status_manual.status].judul} oleh {p.status_manual.oleh_nama}:{' '}
                    {p.status_manual.alasan}
                  </p>
                )}
                {p.peringatan.map((w) => (
                  <p key={w} className="text-danger">
                    {w}
                  </p>
                ))}
              </section>
              <section aria-label={`Risiko per aturan ${p.nama}`} className="flex flex-col gap-(--space-sm)">
                <h3 className="font-semibold">Item berisiko per aturan</h3>
                <dl className="grid grid-cols-[1fr_auto] gap-x-(--space-sm)">
                  {RISK_RULES.map((r) => (
                    <div key={r.kode} className="contents">
                      <dt>{r.label}</dt>
                      <dd className="text-right tabular-nums">{p.per_aturan[r.kode]}</dd>
                    </div>
                  ))}
                </dl>
              </section>
              <section aria-label={`Item berisiko ${p.nama}`} className="flex flex-col gap-(--space-sm)">
                <h3 className="font-semibold">Item berisiko</h3>
                <RiskItems projectId={p.id} />
              </section>
            </div>
          </td>
        </tr>
      )}
    </>
  )
}
