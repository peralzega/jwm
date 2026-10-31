import type { DashboardPanels } from '../api/types'
import { formatDate } from '../board/format'
import { Button, Panel } from '../components/ui'
import { downloadCSV, formatRupiah } from './export'

const pct = (n: number, total: number) => (total > 0 ? Math.round((100 * n) / total) : 0)

function CsvButton({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <Button className="print:hidden" aria-label={`Unduh CSV ${label}`} onClick={onClick}>
      Unduh CSV
    </Button>
  )
}

/** Bilah horizontal; nilai juga tertulis sebagai teks di sampingnya. */
function Bar({ value, max, warna = 'biru' }: { value: number; max: number; warna?: string }) {
  return (
    <span className="h-(--bar-h) min-w-0 flex-1 overflow-hidden rounded-full bg-canvas" aria-hidden="true">
      <span className={`tag-${warna} tag-dot block h-full rounded-full`} style={{ width: `${pct(value, max)}%` }} />
    </span>
  )
}

export function StatusPanel({ data }: { data: DashboardPanels }) {
  const rows = [...data.status.map((s) => ({ ...s })), ...(data.tanpa_status ? [{ label: 'Tanpa status', warna: 'abu', jumlah: data.tanpa_status }] : [])]
  return (
    <Panel
      title="Sebaran status"
      actions={
        <CsvButton
          label="sebaran status"
          onClick={() =>
            downloadCSV('sebaran-status.csv', ['Status', 'Jumlah item', 'Persen'], rows.map((r) => [r.label, r.jumlah, pct(r.jumlah, data.jumlah_item)]))
          }
        />
      }
    >
      {data.jumlah_item === 0 ? (
        <p className="text-muted-foreground">Belum ada item.</p>
      ) : (
        <ul className="flex flex-col gap-(--space-sm)">
          {rows.map((r) => (
            <li key={r.label} className="gap-d grid grid-cols-[minmax(0,var(--chart-label-w))_1fr_auto] items-center">
              <span className="truncate" title={r.label}>
                {r.label}
              </span>
              <Bar value={r.jumlah} max={data.jumlah_item} warna={r.warna} />
              <span className="text-right tabular-nums">
                {r.jumlah} ({pct(r.jumlah, data.jumlah_item)}%)
              </span>
            </li>
          ))}
        </ul>
      )}
    </Panel>
  )
}

export function WorkloadPanel({ data }: { data: DashboardPanels }) {
  const max = Math.max(1, ...data.beban.map((w) => w.item_terbuka))
  return (
    <Panel
      title="Beban kerja per orang"
      actions={
        <CsvButton
          label="beban kerja"
          onClick={() =>
            downloadCSV(
              'beban-kerja.csv',
              ['Nama', 'Item terbuka', 'Hari kerja', 'Terlambat'],
              data.beban.map((w) => [w.nama, w.item_terbuka, w.hari_kerja, w.terlambat]),
            )
          }
        />
      }
    >
      {data.beban.length === 0 ? (
        <p className="text-muted-foreground">Tidak ada item terbuka yang ditugaskan.</p>
      ) : (
        <table className="w-full text-left">
          <thead>
            <tr className="row-h border-b border-line text-muted-foreground">
              <th className="px-d font-medium">Nama</th>
              <th className="px-d font-medium">Item terbuka</th>
              <th className="px-d text-right font-medium">Hari kerja</th>
              <th className="px-d text-right font-medium">Terlambat</th>
            </tr>
          </thead>
          <tbody>
            {data.beban.map((w) => (
              <tr key={w.user_id} className="row-h border-b border-line">
                <td className="px-d">{w.nama}</td>
                <td className="px-d">
                  <span className="gap-d flex items-center">
                    <Bar value={w.item_terbuka} max={max} />
                    <span className="tabular-nums">{w.item_terbuka}</span>
                  </span>
                </td>
                <td className="px-d text-right tabular-nums">{w.hari_kerja}</td>
                <td className={`px-d text-right tabular-nums ${w.terlambat > 0 ? 'text-danger' : ''}`}>{w.terlambat}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {data.belum_ditugaskan > 0 && (
        <p className="text-muted-foreground">{data.belum_ditugaskan} item terbuka belum punya penanggung jawab.</p>
      )}
    </Panel>
  )
}

export function TrendPanel({ data }: { data: DashboardPanels }) {
  const max = Math.max(1, ...data.tren.map((w) => w.selesai))
  const total = data.tren.reduce((n, w) => n + w.selesai, 0)
  return (
    <Panel
      title="Tren penyelesaian"
      actions={
        <CsvButton
          label="tren penyelesaian"
          onClick={() => downloadCSV('tren-penyelesaian.csv', ['Minggu mulai', 'Item selesai'], data.tren.map((w) => [w.minggu_mulai, w.selesai]))}
        />
      }
    >
      <p className="text-muted-foreground">
        {total} item selesai dalam {data.tren.length} minggu terakhir (per minggu, mulai Senin).
      </p>
      <ol aria-label="Item selesai per minggu" className="gap-(--space-xs) flex h-(--chart-h) items-end">
        {data.tren.map((w) => (
          <li
            key={w.minggu_mulai}
            className="flex h-full min-w-0 flex-1 flex-col items-center justify-end gap-(--space-xs)"
            aria-label={`Minggu ${formatDate(w.minggu_mulai)}: ${w.selesai} item`}
          >
            <span className="text-(length:--font-xs) tabular-nums" aria-hidden="true">
              {w.selesai > 0 ? w.selesai : ''}
            </span>
            <span className="w-full rounded-t bg-primary" style={{ height: `${pct(w.selesai, max)}%` }} aria-hidden="true" />
          </li>
        ))}
      </ol>
      <div aria-hidden="true" className="flex justify-between text-(length:--font-xs) text-muted-foreground">
        <span>{formatDate(data.tren[0]?.minggu_mulai ?? null, true)}</span>
        <span>{formatDate(data.tren[data.tren.length - 1]?.minggu_mulai ?? null, true)}</span>
      </div>
    </Panel>
  )
}

export function FinancePanel({ data }: { data: DashboardPanels }) {
  return (
    <Panel
      title="Keuangan"
      actions={
        <CsvButton
          label="keuangan"
          onClick={() =>
            downloadCSV(
              'keuangan.csv',
              ['Proyek', 'Anggaran', 'Realisasi', 'Sisa', 'Jumlah entri'],
              data.keuangan.map((f) => [f.nama, Number(f.anggaran), Number(f.realisasi), Number(f.sisa), f.jumlah_entri]),
            )
          }
        />
      }
    >
      {data.keuangan.length === 0 ? (
        <p className="text-muted-foreground">Tidak ada proyek.</p>
      ) : (
        <table className="w-full text-left">
          <thead>
            <tr className="row-h border-b border-line text-muted-foreground">
              <th className="px-d font-medium">Proyek</th>
              <th className="px-d text-right font-medium">Anggaran</th>
              <th className="px-d text-right font-medium">Realisasi</th>
              <th className="px-d text-right font-medium">Sisa</th>
            </tr>
          </thead>
          <tbody>
            {data.keuangan.map((f) => (
              <tr key={f.project_id} className="row-h border-b border-line">
                <td className="px-d">{f.nama}</td>
                <td className="px-d text-right tabular-nums">{formatRupiah(f.anggaran)}</td>
                <td className="px-d text-right tabular-nums">{formatRupiah(f.realisasi)}</td>
                <td className={`px-d text-right tabular-nums ${Number(f.sisa) < 0 ? 'text-danger' : ''}`}>{formatRupiah(f.sisa)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <p className="text-muted-foreground">Sisa = total anggaran − total realisasi, dari entri keuangan tiap proyek.</p>
    </Panel>
  )
}
