import { useId, useState, type FormEvent } from 'react'

import type {
  ActionKind,
  AutomationAction,
  AutomationCondition,
  AutomationRule,
  AutomationTrigger,
  BoardDetail,
  ColumnType,
  ConditionKind,
  Member,
  TriggerKind,
} from '../../api/types'
import { Alert, Button } from '../../components/ui'
import { ValueInput } from './ValueInput'

export type RuleDraft = Pick<AutomationRule, 'nama' | 'pemicu' | 'kondisi' | 'aksi' | 'aktif'>

const FIELD = 'row-h px-d rounded border border-line bg-surface'

const TRIGGERS: { id: TriggerKind; label: string }[] = [
  { id: 'status_berubah', label: 'Status berubah' },
  { id: 'item_dibuat', label: 'Item dibuat' },
  { id: 'penugasan_berubah', label: 'Penanggung jawab berubah' },
  { id: 'tanggal_tiba', label: 'Tanggal tiba' },
]
const CONDITIONS: { id: ConditionKind; label: string }[] = [
  { id: 'kolom_sama', label: 'Nilai kolom sama dengan' },
  { id: 'person_termasuk', label: 'Penanggung jawab termasuk' },
  { id: 'di_grup', label: 'Item berada di grup' },
]
const ACTIONS: { id: ActionKind; label: string }[] = [
  { id: 'pindah_grup', label: 'Pindahkan ke grup' },
  { id: 'ubah_kolom', label: 'Ubah nilai kolom' },
]
// Sama dengan backend: timeline tidak diubah otomasi (pergeseran successor).
const VALUE_TYPES: ColumnType[] = ['status', 'priority', 'person', 'text', 'number', 'progress', 'date']
const COND_TYPES: ColumnType[] = ['status', 'priority', 'text', 'number', 'progress', 'date']

export function emptyDraft(board: BoardDetail): RuleDraft {
  return {
    nama: '',
    pemicu: { jenis: 'status_berubah' },
    kondisi: [],
    aksi: [{ jenis: 'pindah_grup', group_id: board.grup[0]?.id }],
    aktif: true,
  }
}

function TriggerFields({ board, t, onChange }: { board: BoardDetail; t: AutomationTrigger; onChange: (t: AutomationTrigger) => void }) {
  const statusCols = board.kolom.filter((c) => c.tipe === 'status')
  const labels = (t.column_id ? statusCols.filter((c) => c.id === t.column_id) : statusCols).flatMap((c) => c.konfigurasi.labels ?? [])
  const uniq = [...new Map(labels.map((l) => [l.id, l])).values()]
  switch (t.jenis) {
    case 'status_berubah':
      return (
        <>
          <select aria-label="Kolom status" className={FIELD} value={t.column_id ?? ''} onChange={(e) => onChange({ ...t, column_id: e.target.value || undefined })}>
            <option value="">Kolom status mana pun</option>
            {statusCols.map((c) => (
              <option key={c.id} value={c.id}>
                {c.judul}
              </option>
            ))}
          </select>
          {(['dari', 'ke'] as const).map((k) => (
            <select
              key={k}
              aria-label={k === 'dari' ? 'Dari status' : 'Menjadi status'}
              className={FIELD}
              value={t[k] === undefined ? '*' : t[k]}
              onChange={(e) => onChange({ ...t, [k]: e.target.value === '*' ? undefined : e.target.value })}
            >
              <option value="*">{k === 'dari' ? 'Dari status apa pun' : 'Menjadi status apa pun'}</option>
              <option value="">{k === 'dari' ? 'Dari kosong' : 'Menjadi kosong'}</option>
              {uniq.map((l) => (
                <option key={l.id} value={l.id}>
                  {l.judul}
                </option>
              ))}
            </select>
          ))}
        </>
      )
    case 'penugasan_berubah':
      return (
        <select aria-label="Kolom penanggung jawab" className={FIELD} value={t.column_id ?? ''} onChange={(e) => onChange({ ...t, column_id: e.target.value || undefined })}>
          <option value="">Kolom person mana pun</option>
          {board.kolom
            .filter((c) => c.tipe === 'person')
            .map((c) => (
              <option key={c.id} value={c.id}>
                {c.judul}
              </option>
            ))}
        </select>
      )
    case 'tanggal_tiba':
      return (
        <select aria-label="Kolom tanggal" className={FIELD} value={t.column_id ?? ''} onChange={(e) => onChange({ ...t, column_id: e.target.value || undefined })}>
          <option value="">Pilih kolom tanggal atau timeline (selesai)</option>
          {board.kolom
            .filter((c) => c.tipe === 'date' || c.tipe === 'timeline')
            .map((c) => (
              <option key={c.id} value={c.id}>
                {c.judul}
              </option>
            ))}
        </select>
      )
    default:
      return null
  }
}

function ColumnSelect({
  board,
  types,
  value,
  label,
  onChange,
}: {
  board: BoardDetail
  types: ColumnType[]
  value?: string
  label: string
  onChange: (id: string) => void
}) {
  return (
    <select aria-label={label} className={FIELD} value={value ?? ''} onChange={(e) => onChange(e.target.value)}>
      <option value="">Pilih kolom</option>
      {board.kolom
        .filter((c) => types.includes(c.tipe))
        .map((c) => (
          <option key={c.id} value={c.id}>
            {c.judul}
          </option>
        ))}
    </select>
  )
}

function GroupSelect({ board, value, label, onChange }: { board: BoardDetail; value?: string; label: string; onChange: (id: string) => void }) {
  return (
    <select aria-label={label} className={FIELD} value={value ?? ''} onChange={(e) => onChange(e.target.value)}>
      <option value="">Pilih grup</option>
      {board.grup.map((g) => (
        <option key={g.id} value={g.id}>
          {g.judul}
        </option>
      ))}
    </select>
  )
}

/** Formulir aturan: pemicu -> kondisi -> aksi, tanpa menulis kode. */
export function RuleForm({
  board,
  members,
  initial,
  pending,
  error,
  onSubmit,
  onCancel,
}: {
  board: BoardDetail
  members: Member[]
  initial: RuleDraft
  pending: boolean
  error: string | null
  onSubmit: (d: RuleDraft) => void
  onCancel: () => void
}) {
  const [d, setD] = useState<RuleDraft>(initial)
  const ids = { nama: useId(), pemicu: useId() }
  const setCond = (i: number, c: AutomationCondition) => setD({ ...d, kondisi: d.kondisi.map((x, j) => (j === i ? c : x)) })
  const setAct = (i: number, a: AutomationAction) => setD({ ...d, aksi: d.aksi.map((x, j) => (j === i ? a : x)) })

  function submit(e: FormEvent) {
    e.preventDefault()
    onSubmit(d)
  }

  return (
    <form onSubmit={submit} aria-label="Aturan otomasi" className="gap-d flex flex-col">
      <label htmlFor={ids.nama} className="flex flex-col gap-(--space-sm)">
        Nama aturan
        <input id={ids.nama} required maxLength={200} className={FIELD} value={d.nama} onChange={(e) => setD({ ...d, nama: e.target.value })} />
      </label>

      <fieldset className="gap-d flex flex-col rounded border border-line p-(--space-sm)">
        <legend className="px-(--space-sm) font-semibold">1. Pemicu</legend>
        <div className="gap-d flex flex-wrap items-center">
          <select
            id={ids.pemicu}
            aria-label="Jenis pemicu"
            className={FIELD}
            value={d.pemicu.jenis}
            onChange={(e) => setD({ ...d, pemicu: { jenis: e.target.value as TriggerKind } })}
          >
            {TRIGGERS.map((t) => (
              <option key={t.id} value={t.id}>
                {t.label}
              </option>
            ))}
          </select>
          <TriggerFields board={board} t={d.pemicu} onChange={(pemicu) => setD({ ...d, pemicu })} />
        </div>
      </fieldset>

      <fieldset className="gap-d flex flex-col rounded border border-line p-(--space-sm)">
        <legend className="px-(--space-sm) font-semibold">2. Kondisi (semua harus terpenuhi)</legend>
        {d.kondisi.length === 0 && <p className="text-muted-foreground">Tanpa kondisi: aturan selalu berjalan saat pemicu terjadi.</p>}
        {d.kondisi.map((c, i) => {
          const n = i + 1
          return (
            <div key={i} className="gap-d flex flex-wrap items-center">
              <select
                aria-label={`Jenis kondisi ${n}`}
                className={FIELD}
                value={c.jenis}
                onChange={(e) => setCond(i, { jenis: e.target.value as ConditionKind })}
              >
                {CONDITIONS.map((x) => (
                  <option key={x.id} value={x.id}>
                    {x.label}
                  </option>
                ))}
              </select>
              {c.jenis === 'kolom_sama' && (
                <>
                  <ColumnSelect board={board} types={COND_TYPES} value={c.column_id} label={`Kolom kondisi ${n}`} onChange={(id) => setCond(i, { ...c, column_id: id, nilai: null })} />
                  <ColumnValue board={board} columnId={c.column_id} value={c.nilai} members={members} label={`Nilai kondisi ${n}`} onChange={(v) => setCond(i, { ...c, nilai: v })} />
                </>
              )}
              {c.jenis === 'person_termasuk' && (
                <>
                  <ColumnSelect board={board} types={['person']} value={c.column_id} label={`Kolom kondisi ${n}`} onChange={(id) => setCond(i, { ...c, column_id: id })} />
                  <select aria-label={`Pengguna kondisi ${n}`} className={FIELD} value={c.user_id ?? ''} onChange={(e) => setCond(i, { ...c, user_id: e.target.value })}>
                    <option value="">Pilih pengguna</option>
                    {members.map((m) => (
                      <option key={m.user_id} value={m.user_id}>
                        {m.nama}
                      </option>
                    ))}
                  </select>
                </>
              )}
              {c.jenis === 'di_grup' && (
                <GroupSelect board={board} value={c.group_id} label={`Grup kondisi ${n}`} onChange={(id) => setCond(i, { ...c, group_id: id })} />
              )}
              <Button aria-label={`Hapus kondisi ${n}`} onClick={() => setD({ ...d, kondisi: d.kondisi.filter((_, j) => j !== i) })}>
                Hapus
              </Button>
            </div>
          )
        })}
        <div>
          <Button disabled={d.kondisi.length >= 10} onClick={() => setD({ ...d, kondisi: [...d.kondisi, { jenis: 'kolom_sama' }] })}>
            Tambah kondisi
          </Button>
        </div>
      </fieldset>

      <fieldset className="gap-d flex flex-col rounded border border-line p-(--space-sm)">
        <legend className="px-(--space-sm) font-semibold">3. Aksi (dijalankan berurutan)</legend>
        {d.aksi.map((a, i) => {
          const n = i + 1
          return (
            <div key={i} className="gap-d flex flex-wrap items-center">
              <select aria-label={`Jenis aksi ${n}`} className={FIELD} value={a.jenis} onChange={(e) => setAct(i, { jenis: e.target.value as ActionKind })}>
                {ACTIONS.map((x) => (
                  <option key={x.id} value={x.id}>
                    {x.label}
                  </option>
                ))}
              </select>
              {a.jenis === 'pindah_grup' && (
                <GroupSelect board={board} value={a.group_id} label={`Grup aksi ${n}`} onChange={(id) => setAct(i, { ...a, group_id: id })} />
              )}
              {a.jenis === 'ubah_kolom' && (
                <>
                  <ColumnSelect board={board} types={VALUE_TYPES} value={a.column_id} label={`Kolom aksi ${n}`} onChange={(id) => setAct(i, { ...a, column_id: id, nilai: null })} />
                  <ColumnValue board={board} columnId={a.column_id} value={a.nilai} members={members} label={`Nilai aksi ${n}`} onChange={(v) => setAct(i, { ...a, nilai: v })} />
                </>
              )}
              <Button aria-label={`Hapus aksi ${n}`} disabled={d.aksi.length <= 1} onClick={() => setD({ ...d, aksi: d.aksi.filter((_, j) => j !== i) })}>
                Hapus
              </Button>
            </div>
          )
        })}
        <div>
          <Button disabled={d.aksi.length >= 10} onClick={() => setD({ ...d, aksi: [...d.aksi, { jenis: 'ubah_kolom' }] })}>
            Tambah aksi
          </Button>
        </div>
      </fieldset>

      <label className="gap-d flex items-center">
        <input type="checkbox" checked={d.aktif} onChange={(e) => setD({ ...d, aktif: e.target.checked })} />
        Aktifkan aturan
      </label>
      {error && <Alert>{error}</Alert>}
      <div className="gap-d flex">
        <Button type="submit" variant="primary" disabled={pending || !d.nama.trim()}>
          Simpan aturan
        </Button>
        <Button onClick={onCancel}>Batal</Button>
      </div>
    </form>
  )
}

/** Isian nilai untuk kolom terpilih; kosong bila kolom belum dipilih. */
function ColumnValue({
  board,
  columnId,
  ...props
}: { board: BoardDetail; columnId?: string } & Omit<Parameters<typeof ValueInput>[0], 'col'>) {
  const col = board.kolom.find((c) => c.id === columnId)
  return col ? <ValueInput col={col} {...props} /> : null
}
