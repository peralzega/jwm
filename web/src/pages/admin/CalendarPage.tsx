import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useId, useState, type FormEvent } from 'react'

import { api, errorMessage } from '../../api/client'
import type { CalendarDay, CalendarYear } from '../../api/types'
import { Alert, Button, Loading, Panel, TextField } from '../../components/ui'
import { ALL_SCHEDULES_KEY } from '../../board/deps/queries'

const CALENDAR_KEY = ['work-calendar'] as const
// Batas badan JSON server 1 MB; sisakan ruang untuk pembungkus JSON.
const MAX_CSV_BYTES = 900 * 1024

const dayFmt = new Intl.DateTimeFormat('id-ID', { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric', timeZone: 'UTC' })
const formatDay = (iso: string) => dayFmt.format(new Date(`${iso}T00:00:00Z`))

function useCalendarYear(tahun: number) {
  return useQuery({
    queryKey: [...CALENDAR_KEY, tahun],
    queryFn: () => api<CalendarYear>('GET', `/work-calendar?tahun=${tahun}`),
  })
}

/** Setelah kalender berubah, hasil CPM semua board dihitung ulang saat dibuka lagi. */
function useCalendarChanged() {
  const qc = useQueryClient()
  return () => {
    void qc.invalidateQueries({ queryKey: CALENDAR_KEY })
    void qc.invalidateQueries({ queryKey: ALL_SCHEDULES_KEY })
  }
}

function DayForm() {
  const changed = useCalendarChanged()
  const [tanggal, setTanggal] = useState('')
  const [keterangan, setKeterangan] = useState('')
  const [hariKerja, setHariKerja] = useState(false)
  const kerjaId = useId()
  const save = useMutation({
    mutationFn: () => api<CalendarDay>('PUT', `/admin/work-calendar/${tanggal}`, { hari_kerja: hariKerja, keterangan }),
    onSuccess: () => {
      setTanggal('')
      setKeterangan('')
      setHariKerja(false)
      changed()
    },
  })

  function submit(e: FormEvent) {
    e.preventDefault()
    if (tanggal) save.mutate()
  }

  return (
    <form onSubmit={submit} aria-label="Tambah atau ubah tanggal" className="gap-d grid grid-cols-1 items-end md:grid-cols-4">
      <TextField
        label="Tanggal"
        type="date"
        value={tanggal}
        onChange={(e) => setTanggal(e.target.value)}
        required
      />
      <TextField label="Keterangan" value={keterangan} maxLength={200} onChange={(e) => setKeterangan(e.target.value)} />
      <label htmlFor={kerjaId} className="row-h gap-d flex items-center">
        <input id={kerjaId} type="checkbox" checked={hariKerja} onChange={(e) => setHariKerja(e.target.checked)} />
        Hari kerja (mis. Sabtu masuk)
      </label>
      <Button type="submit" variant="primary" disabled={!tanggal || save.isPending}>
        Simpan tanggal
      </Button>
      {save.isError && (
        <div className="md:col-span-4">
          <Alert>{errorMessage(save.error)}</Alert>
        </div>
      )}
    </form>
  )
}

function ImportForm() {
  const changed = useCalendarChanged()
  const [file, setFile] = useState<File | null>(null)
  const [ganti, setGanti] = useState(false)
  const [localError, setLocalError] = useState<string | null>(null)
  const fileId = useId()
  const gantiId = useId()
  const hintId = useId()
  const imp = useMutation({
    mutationFn: async () => {
      if (!file) throw new Error('Pilih berkas CSV.')
      const csv = await file.text()
      return api<{ jumlah: number; tahun: number[]; dihapus: number }>('POST', '/admin/work-calendar/import', { csv, ganti })
    },
    onSuccess: changed,
  })

  function submit(e: FormEvent) {
    e.preventDefault()
    setLocalError(null)
    if (file && file.size > MAX_CSV_BYTES) {
      setLocalError('Berkas terlalu besar. Pecah per tahun lalu impor satu per satu.')
      return
    }
    imp.mutate()
  }

  return (
    <form onSubmit={submit} aria-label="Impor CSV" className="gap-d flex flex-col">
      <p id={hintId} className="text-muted-foreground">
        Baris pertama berisi nama kolom: <code>tanggal</code> (YYYY-MM-DD), <code>keterangan</code>, dan opsional{' '}
        <code>hari_kerja</code> (ya atau tidak; kosong berarti libur). Pemisah koma atau titik koma. Berkas dengan satu
        baris salah ditolak seluruhnya, lengkap dengan nomor barisnya.
      </p>
      <div className="gap-d flex flex-wrap items-center">
        <label htmlFor={fileId} className="font-medium">
          Berkas CSV
        </label>
        <input
          id={fileId}
          type="file"
          accept=".csv,text/csv"
          aria-describedby={hintId}
          onChange={(e) => setFile(e.target.files?.[0] ?? null)}
        />
        <label htmlFor={gantiId} className="gap-d flex items-center">
          <input id={gantiId} type="checkbox" checked={ganti} onChange={(e) => setGanti(e.target.checked)} />
          Ganti seluruh isi tahun yang ada di berkas
        </label>
        <Button type="submit" variant="primary" disabled={!file || imp.isPending}>
          Impor
        </Button>
      </div>
      {localError && <Alert>{localError}</Alert>}
      {imp.isError && <Alert>{errorMessage(imp.error)}</Alert>}
      {imp.isSuccess && (
        <Alert kind="ok">
          {imp.data.jumlah} tanggal diimpor untuk tahun {imp.data.tahun.join(', ')}
          {imp.data.dihapus > 0 && `; ${imp.data.dihapus} tanggal lama dihapus`}.
        </Alert>
      )}
    </form>
  )
}

function DayRow({ day }: { day: CalendarDay }) {
  const changed = useCalendarChanged()
  const remove = useMutation({
    mutationFn: () => api<undefined>('DELETE', `/admin/work-calendar/${day.tanggal}`),
    onSuccess: changed,
  })
  return (
    <tr className="row-h border-b border-line">
      <td className="px-d">{formatDay(day.tanggal)}</td>
      <td className="px-d">{day.hari_kerja ? 'Hari kerja' : 'Libur'}</td>
      <td className="px-d">{day.keterangan}</td>
      <td className="px-d">
        <Button
          variant="danger"
          aria-label={`Hapus ${formatDay(day.tanggal)}`}
          disabled={remove.isPending}
          onClick={() => {
            if (window.confirm(`Hapus ${formatDay(day.tanggal)} dari kalender kerja?`)) remove.mutate()
          }}
        >
          Hapus
        </Button>
        {remove.isError && <span className="px-d text-danger">{errorMessage(remove.error)}</span>}
      </td>
    </tr>
  )
}

/** Kalender kerja (Admin): Sabtu dan Minggu libur otomatis; tanggal lain diisi dari data resmi. */
export function CalendarPage() {
  const [tahun, setTahun] = useState(() => new Date().getFullYear())
  const year = useCalendarYear(tahun)
  const tahunId = useId()

  return (
    <div className="gap-d flex flex-col">
      <h1 className="text-(length:--heading-size) font-semibold">Kalender kerja</h1>
      <p className="text-muted-foreground">
        Sabtu dan Minggu otomatis libur. Isi hari libur lain (dan akhir pekan yang dijadikan hari kerja) dari kalender
        resmi. Aplikasi tidak menyediakan daftar libur bawaan.
      </p>
      <Panel title="Impor CSV">
        <ImportForm />
      </Panel>
      <Panel title="Tambah atau ubah tanggal">
        <DayForm />
      </Panel>
      <Panel
        title={`Isi kalender ${tahun}`}
        actions={
          <div className="gap-d flex items-center">
            <Button aria-label="Tahun sebelumnya" onClick={() => setTahun((t) => t - 1)}>
              ‹
            </Button>
            <label htmlFor={tahunId} className="sr-only">
              Tahun
            </label>
            <input
              id={tahunId}
              type="number"
              min={1900}
              max={2999}
              className="row-h px-d w-(--dep-lag-w) rounded border border-line bg-surface tabular-nums"
              value={tahun}
              onChange={(e) => {
                const n = Number(e.target.value)
                if (Number.isInteger(n) && n >= 1900 && n <= 2999) setTahun(n)
              }}
            />
            <Button aria-label="Tahun berikutnya" onClick={() => setTahun((t) => t + 1)}>
              ›
            </Button>
          </div>
        }
      >
        {year.isPending && <Loading />}
        {year.isError && <Alert>{errorMessage(year.error)}</Alert>}
        {year.data && (
          <div className="gap-d flex flex-col">
            {!year.data.tahun_terisi.includes(tahun) && (
              <Alert kind="ok">
                Kalender {tahun} belum diisi. Perhitungan jadwal tahun ini hanya menganggap Sabtu dan Minggu sebagai
                libur.
              </Alert>
            )}
            {year.data.tahun_terisi.length > 0 && (
              <p className="text-muted-foreground">Tahun yang sudah terisi: {year.data.tahun_terisi.join(', ')}.</p>
            )}
            {year.data.items.length > 0 && (
              <table className="w-full text-left">
                <thead>
                  <tr className="row-h border-b border-line text-muted-foreground">
                    <th className="px-d font-medium">Tanggal</th>
                    <th className="px-d font-medium">Jenis</th>
                    <th className="px-d font-medium">Keterangan</th>
                    <th className="px-d font-medium">
                      <span className="sr-only">Aksi</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {year.data.items.map((d) => (
                    <DayRow key={d.tanggal} day={d} />
                  ))}
                </tbody>
              </table>
            )}
          </div>
        )}
      </Panel>
    </div>
  )
}
