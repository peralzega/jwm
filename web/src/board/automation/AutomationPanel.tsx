import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'

import { api, errorMessage } from '../../api/client'
import type { AutomationRule, AutomationRun, BoardDetail, Member, WebhookDelivery } from '../../api/types'
import { Alert, Button, Loading, Panel } from '../../components/ui'
import { describeRule } from './describe'
import { automationsKey } from './keys'
import { emptyDraft, RuleForm, type RuleDraft } from './RuleForm'

export { automationsKey } from './keys'

const HASIL: Record<AutomationRun['hasil'], { label: string; cls: string }> = {
  berhasil: { label: 'Berhasil', cls: '' },
  dilewati: { label: 'Dilewati', cls: 'text-muted-foreground' },
  gagal: { label: 'Gagal', cls: 'text-danger' },
  diblokir_loop: { label: 'Diblokir (loop)', cls: 'text-danger' },
}

const timeFmt = new Intl.DateTimeFormat('id-ID', { dateStyle: 'medium', timeStyle: 'short' })

const hasWebhook = (r: AutomationRule) => r.aksi.some((a) => a.jenis === 'panggil_webhook')

const KIRIMAN: Record<WebhookDelivery['status'], { label: string; cls: string }> = {
  menunggu: { label: 'Menunggu / dicoba ulang', cls: 'text-muted-foreground' },
  terkirim: { label: 'Terkirim', cls: '' },
  gagal: { label: 'Gagal', cls: 'text-danger' },
}

/** Rahasia HMAC hanya ditampilkan sekali; penerima (mis. n8n) memakainya untuk verifikasi. */
function SecretNotice({ secret, onClose }: { secret: string; onClose: () => void }) {
  return (
    <Alert kind="ok">
      <div className="gap-(--space-sm) flex flex-col">
        <span>
          Rahasia webhook (simpan sekarang; tidak akan ditampilkan lagi):{' '}
          <code aria-label="Rahasia webhook" className="rounded bg-surface px-(--space-sm) font-mono select-all">
            {secret}
          </code>
        </span>
        <span className="text-muted-foreground">
          Setiap kiriman membawa header <code>X-JWM-Timestamp</code> dan <code>X-JWM-Signature</code> = sha256= HMAC-SHA256
          rahasia atas teks &quot;timestamp.isi&quot;. Tolak kiriman yang tanda tangannya tidak cocok atau timestamp-nya
          lebih dari 5 menit.
        </span>
        <div>
          <Button onClick={onClose}>Tutup</Button>
        </div>
      </div>
    </Alert>
  )
}

function Deliveries({ rule }: { rule: AutomationRule }) {
  const list = useQuery({
    queryKey: ['automation-webhooks', rule.id],
    queryFn: () => api<{ items: WebhookDelivery[] }>('GET', `/automations/${rule.id}/webhooks`).then((r) => r.items),
    refetchInterval: 5000,
  })
  if (list.isPending) return <Loading />
  if (list.isError) return <Alert>{errorMessage(list.error)}</Alert>
  if (list.data.length === 0) return <p className="text-muted-foreground">Belum ada kiriman webhook.</p>
  return (
    <table className="w-full text-left">
      <caption className="sr-only">Kiriman webhook {rule.nama}</caption>
      <thead>
        <tr className="row-h border-b border-line text-muted-foreground">
          <th className="px-d font-medium">Waktu</th>
          <th className="px-d font-medium">Status</th>
          <th className="px-d text-right font-medium">Percobaan</th>
          <th className="px-d font-medium">Keterangan</th>
        </tr>
      </thead>
      <tbody>
        {list.data.map((d) => (
          <tr key={d.id} className="row-h border-b border-line">
            <td className="px-d whitespace-nowrap">{timeFmt.format(new Date(d.dibuat_pada))}</td>
            <td className={`px-d ${KIRIMAN[d.status].cls}`}>{KIRIMAN[d.status].label}</td>
            <td className="px-d text-right tabular-nums">{d.percobaan}</td>
            <td className="px-d">
              {d.kode_http !== null && `HTTP ${d.kode_http}`}
              {d.galat && ` ${d.galat}`}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

function Runs({ rule }: { rule: AutomationRule }) {
  const runs = useQuery({
    queryKey: ['automation-runs', rule.id],
    queryFn: () => api<{ items: AutomationRun[] }>('GET', `/automations/${rule.id}/runs`).then((r) => r.items),
    refetchInterval: 5000,
  })
  if (runs.isPending) return <Loading />
  if (runs.isError) return <Alert>{errorMessage(runs.error)}</Alert>
  if (runs.data.length === 0) return <p className="text-muted-foreground">Aturan ini belum pernah berjalan.</p>
  return (
    <table className="w-full text-left">
      <caption className="sr-only">Riwayat eksekusi {rule.nama}</caption>
      <thead>
        <tr className="row-h border-b border-line text-muted-foreground">
          <th className="px-d font-medium">Waktu</th>
          <th className="px-d font-medium">Item</th>
          <th className="px-d font-medium">Hasil</th>
          <th className="px-d font-medium">Keterangan</th>
        </tr>
      </thead>
      <tbody>
        {runs.data.map((r) => (
          <tr key={r.id} className="row-h border-b border-line">
            <td className="px-d whitespace-nowrap">{timeFmt.format(new Date(r.waktu))}</td>
            <td className="px-d">{r.item_judul ?? <span className="text-muted-foreground">item terhapus</span>}</td>
            <td className={`px-d ${HASIL[r.hasil].cls}`}>{HASIL[r.hasil].label}</td>
            <td className="px-d">{r.pesan}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

/** Tab Otomasi board: daftar aturan, formulir, dan riwayat eksekusi. */
export function AutomationPanel({ board, members }: { board: BoardDetail; members: Member[] }) {
  const qc = useQueryClient()
  const key = automationsKey(board.id)
  const rules = useQuery({
    queryKey: key,
    queryFn: () => api<{ items: AutomationRule[]; bisa_kelola: boolean }>('GET', `/boards/${board.id}/automations`),
  })
  const [editing, setEditing] = useState<AutomationRule | 'baru' | null>(null)
  const [history, setHistory] = useState<string | null>(null)
  const [secret, setSecret] = useState<string | null>(null)
  const refresh = () => qc.invalidateQueries({ queryKey: key })
  const save = useMutation({
    mutationFn: (v: { id?: string; draft: RuleDraft }) =>
      v.id
        ? api<AutomationRule>('PATCH', `/automations/${v.id}`, v.draft)
        : api<AutomationRule>('POST', `/boards/${board.id}/automations`, v.draft),
    onSuccess: (saved) => {
      setEditing(null)
      if (saved.webhook_rahasia) setSecret(saved.webhook_rahasia)
      void refresh()
    },
  })
  const rotate = useMutation({
    mutationFn: (id: string) => api<{ webhook_rahasia: string }>('POST', `/automations/${id}/webhook-secret`),
    onSuccess: (r) => setSecret(r.webhook_rahasia),
  })
  const toggle = useMutation({
    mutationFn: (r: AutomationRule) =>
      api<AutomationRule>('PATCH', `/automations/${r.id}`, {
        nama: r.nama,
        pemicu: r.pemicu,
        kondisi: r.kondisi,
        aksi: r.aksi,
        aktif: !r.aktif,
      }),
    onSuccess: refresh,
  })
  const remove = useMutation({
    mutationFn: (id: string) => api<undefined>('DELETE', `/automations/${id}`),
    onSuccess: refresh,
  })
  const ctx = { board, members }

  if (rules.isPending) return <Loading />
  if (rules.isError) return <Alert>{errorMessage(rules.error)}</Alert>
  const canManage = rules.data.bisa_kelola

  return (
    <div className="gap-d flex flex-col">
      <p className="text-muted-foreground">
        Aturan dijalankan sistem beberapa detik setelah perubahan tersimpan. Satu aturan paling banyak berjalan 3 kali
        per item dalam satu rantai otomasi.
      </p>
      {(toggle.isError || remove.isError || rotate.isError) && (
        <Alert>{errorMessage(toggle.error ?? remove.error ?? rotate.error)}</Alert>
      )}
      {secret && <SecretNotice secret={secret} onClose={() => setSecret(null)} />}
      {canManage && editing === null && (
        <div>
          <Button variant="primary" onClick={() => setEditing('baru')}>
            Tambah aturan
          </Button>
        </div>
      )}
      {editing !== null && (
        <Panel title={editing === 'baru' ? 'Aturan baru' : `Ubah: ${editing.nama}`}>
          <RuleForm
            key={editing === 'baru' ? 'baru' : editing.id}
            board={board}
            members={members}
            initial={editing === 'baru' ? emptyDraft(board) : editing}
            pending={save.isPending}
            error={save.isError ? errorMessage(save.error) : null}
            onSubmit={(draft) => save.mutate({ id: editing === 'baru' ? undefined : editing.id, draft })}
            onCancel={() => {
              save.reset()
              setEditing(null)
            }}
          />
        </Panel>
      )}
      {rules.data.items.length === 0 ? (
        <p className="text-muted-foreground">Belum ada aturan otomasi di board ini.</p>
      ) : (
        <ul aria-label="Aturan otomasi" className="gap-d flex flex-col">
          {rules.data.items.map((r) => (
            <li key={r.id} className="gap-(--space-sm) flex flex-col rounded border border-line bg-surface p-(--space-sm)">
              <div className="gap-d flex flex-wrap items-center justify-between">
                <span className="font-semibold">
                  {r.nama}
                  {!r.aktif && <span className="font-normal text-muted-foreground"> (nonaktif)</span>}
                </span>
                <span className="gap-(--space-sm) flex flex-wrap">
                  <Button aria-expanded={history === r.id} aria-label={`Riwayat ${r.nama}`} onClick={() => setHistory(history === r.id ? null : r.id)}>
                    Riwayat
                  </Button>
                  {canManage && (
                    <>
                      <Button aria-label={`${r.aktif ? 'Nonaktifkan' : 'Aktifkan'} ${r.nama}`} disabled={toggle.isPending} onClick={() => toggle.mutate(r)}>
                        {r.aktif ? 'Nonaktifkan' : 'Aktifkan'}
                      </Button>
                      <Button aria-label={`Ubah ${r.nama}`} onClick={() => setEditing(r)}>
                        Ubah
                      </Button>
                      {hasWebhook(r) && (
                        <Button
                          aria-label={`Ganti rahasia webhook ${r.nama}`}
                          disabled={rotate.isPending}
                          onClick={() => {
                            if (window.confirm('Ganti rahasia webhook? Penerima harus memakai rahasia baru.')) rotate.mutate(r.id)
                          }}
                        >
                          Ganti rahasia
                        </Button>
                      )}
                      <Button
                        variant="danger"
                        aria-label={`Hapus ${r.nama}`}
                        disabled={remove.isPending}
                        onClick={() => {
                          if (window.confirm(`Hapus aturan "${r.nama}" beserta riwayatnya?`)) remove.mutate(r.id)
                        }}
                      >
                        Hapus
                      </Button>
                    </>
                  )}
                </span>
              </div>
              <p>{describeRule(ctx, r)}</p>
              {history === r.id && <Runs rule={r} />}
              {history === r.id && hasWebhook(r) && <Deliveries rule={r} />}
            </li>
          ))}
        </ul>
      )}
      {!canManage && <p className="text-muted-foreground">Hanya manager proyek yang dapat mengubah aturan.</p>}
    </div>
  )
}
