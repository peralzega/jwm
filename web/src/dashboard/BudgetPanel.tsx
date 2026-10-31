import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useId, useState, type FormEvent } from 'react'

import { api, errorMessage } from '../api/client'
import type { Budget, BudgetEntry, BudgetKind } from '../api/types'
import { formatDate } from '../board/format'
import { Alert, Button, Panel } from '../components/ui'
import { formatRupiah } from './export'
import { useLiveUpdates } from './live'

export const budgetKey = (projectId: string) => ['budget', projectId] as const

const KIND_LABEL: Record<BudgetKind, string> = { anggaran: 'Anggaran', realisasi: 'Realisasi' }

// Rupiah tanpa pemisah ribuan, paling banyak dua desimal (titik atau koma).
const AMOUNT = /^\d{1,16}([.,]\d{1,2})?$/

type Draft = { tanggal: string; jenis: BudgetKind; jumlah: string; keterangan: string }

const today = () => new Date().toLocaleDateString('sv-SE')

function EntryForm({
  projectId,
  editing,
  onDone,
}: {
  projectId: string
  editing: BudgetEntry | null
  onDone: () => void
}) {
  const qc = useQueryClient()
  const [d, setD] = useState<Draft>(() =>
    editing
      ? { tanggal: editing.tanggal, jenis: editing.jenis, jumlah: editing.jumlah, keterangan: editing.keterangan }
      : { tanggal: today(), jenis: 'realisasi', jumlah: '', keterangan: '' },
  )
  const ids = { tanggal: useId(), jenis: useId(), jumlah: useId(), ket: useId(), hint: useId() }
  const valid = !!d.tanggal && AMOUNT.test(d.jumlah.trim()) && Number(d.jumlah.replace(',', '.')) > 0
  const save = useMutation({
    mutationFn: () => {
      const body = { ...d, jumlah: d.jumlah.trim().replace(',', '.') }
      return editing
        ? api<undefined>('PATCH', `/budget-entries/${editing.id}`, body)
        : api<{ id: string }>('POST', `/projects/${projectId}/budget`, body)
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: budgetKey(projectId) })
      if (!editing) setD((x) => ({ ...x, jumlah: '', keterangan: '' }))
      onDone()
    },
  })

  function submit(e: FormEvent) {
    e.preventDefault()
    if (valid) save.mutate()
  }

  const field = 'row-h px-d rounded border border-line bg-surface'
  return (
    <form
      onSubmit={submit}
      aria-label={editing ? 'Ubah entri keuangan' : 'Tambah entri keuangan'}
      className="gap-d grid grid-cols-1 items-end md:grid-cols-5"
    >
      <label htmlFor={ids.tanggal} className="flex flex-col gap-(--space-sm)">
        Tanggal
        <input id={ids.tanggal} type="date" required className={field} value={d.tanggal} onChange={(e) => setD({ ...d, tanggal: e.target.value })} />
      </label>
      <label htmlFor={ids.jenis} className="flex flex-col gap-(--space-sm)">
        Jenis
        <select id={ids.jenis} className={field} value={d.jenis} onChange={(e) => setD({ ...d, jenis: e.target.value as BudgetKind })}>
          <option value="anggaran">Anggaran</option>
          <option value="realisasi">Realisasi</option>
        </select>
      </label>
      <label htmlFor={ids.jumlah} className="flex flex-col gap-(--space-sm)">
        Jumlah (Rp)
        <input
          id={ids.jumlah}
          inputMode="decimal"
          aria-describedby={ids.hint}
          aria-invalid={d.jumlah !== '' && !valid}
          className={`${field} tabular-nums`}
          value={d.jumlah}
          onChange={(e) => setD({ ...d, jumlah: e.target.value })}
        />
      </label>
      <label htmlFor={ids.ket} className="flex flex-col gap-(--space-sm)">
        Keterangan
        <input id={ids.ket} maxLength={500} className={field} value={d.keterangan} onChange={(e) => setD({ ...d, keterangan: e.target.value })} />
      </label>
      <div className="gap-d flex">
        <Button type="submit" variant="primary" disabled={!valid || save.isPending}>
          {editing ? 'Simpan' : 'Tambah'}
        </Button>
        {editing && <Button onClick={onDone}>Batal</Button>}
      </div>
      <p id={ids.hint} className="text-muted-foreground md:col-span-5">
        Tanpa pemisah ribuan, paling banyak dua desimal, mis. 1500000 atau 1500000,50.
      </p>
      {save.isError && (
        <div className="md:col-span-5">
          <Alert>{errorMessage(save.error)}</Alert>
        </div>
      )}
    </form>
  )
}

/** Keuangan proyek: total, daftar entri, dan formulir untuk PMO/Project Manager. */
export function BudgetPanel({ projectId }: { projectId: string }) {
  const qc = useQueryClient()
  const budget = useQuery({
    queryKey: budgetKey(projectId),
    queryFn: () => api<Budget>('GET', `/projects/${projectId}/budget`),
  })
  useLiveUpdates(projectId, [budgetKey(projectId)])
  const [editing, setEditing] = useState<BudgetEntry | null>(null)
  const remove = useMutation({
    mutationFn: (id: string) => api<undefined>('DELETE', `/budget-entries/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: budgetKey(projectId) }),
  })

  return (
    <Panel title="Keuangan">
      {budget.isError && <Alert>{errorMessage(budget.error)}</Alert>}
      {budget.data && (
        <div className="gap-d flex flex-col">
          <dl aria-label="Total keuangan" className="gap-d grid grid-cols-3">
            {(['anggaran', 'realisasi', 'sisa'] as const).map((k) => (
              <div key={k}>
                <dt className="text-muted-foreground">{k === 'sisa' ? 'Sisa' : KIND_LABEL[k]}</dt>
                <dd className={`font-semibold tabular-nums ${k === 'sisa' && Number(budget.data.total.sisa) < 0 ? 'text-danger' : ''}`}>
                  {formatRupiah(budget.data.total[k])}
                </dd>
              </div>
            ))}
          </dl>
          {budget.data.bisa_ubah && (
            <EntryForm key={editing?.id ?? 'baru'} projectId={projectId} editing={editing} onDone={() => setEditing(null)} />
          )}
          {remove.isError && <Alert>{errorMessage(remove.error)}</Alert>}
          {budget.data.items.length === 0 ? (
            <p className="text-muted-foreground">Belum ada entri keuangan.</p>
          ) : (
            <table className="w-full text-left">
              <caption className="sr-only">Entri keuangan</caption>
              <thead>
                <tr className="row-h border-b border-line text-muted-foreground">
                  <th className="px-d font-medium">Tanggal</th>
                  <th className="px-d font-medium">Jenis</th>
                  <th className="px-d text-right font-medium">Jumlah</th>
                  <th className="px-d font-medium">Keterangan</th>
                  <th className="px-d font-medium">Oleh</th>
                  {budget.data.bisa_ubah && (
                    <th className="px-d font-medium">
                      <span className="sr-only">Aksi</span>
                    </th>
                  )}
                </tr>
              </thead>
              <tbody>
                {budget.data.items.map((e) => (
                  <tr key={e.id} className="row-h border-b border-line">
                    <td className="px-d">{formatDate(e.tanggal)}</td>
                    <td className="px-d">{KIND_LABEL[e.jenis]}</td>
                    <td className="px-d text-right tabular-nums">{formatRupiah(e.jumlah)}</td>
                    <td className="px-d">{e.keterangan}</td>
                    <td className="px-d text-muted-foreground">{e.dibuat_oleh_nama}</td>
                    {budget.data.bisa_ubah && (
                      <td className="px-d">
                        <span className="gap-(--space-sm) flex">
                          <Button aria-label={`Ubah entri ${formatDate(e.tanggal)} ${KIND_LABEL[e.jenis]}`} onClick={() => setEditing(e)}>
                            Ubah
                          </Button>
                          <Button
                            variant="danger"
                            aria-label={`Hapus entri ${formatDate(e.tanggal)} ${KIND_LABEL[e.jenis]}`}
                            disabled={remove.isPending}
                            onClick={() => {
                              if (window.confirm('Hapus entri keuangan ini?')) remove.mutate(e.id)
                            }}
                          >
                            Hapus
                          </Button>
                        </span>
                      </td>
                    )}
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}
    </Panel>
  )
}
