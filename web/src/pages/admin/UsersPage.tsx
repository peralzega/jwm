import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'

import { api, errorMessage } from '../../api/client'
import { ROLE_LABEL, type AdminUser, type Role, type UserWithTempPassword } from '../../api/types'
import { useMe } from '../../auth/session'
import { Alert, Button, Loading, Panel, SelectField, TextField } from '../../components/ui'

const USERS_KEY = ['admin', 'users'] as const
const ROLES = Object.keys(ROLE_LABEL) as Role[]

function useUsers(q: string) {
  return useQuery({
    queryKey: [...USERS_KEY, q],
    queryFn: () => api<{ items: AdminUser[] }>('GET', `/admin/users?q=${encodeURIComponent(q)}`),
  })
}

/** Menampilkan sandi sementara satu kali; hilang setelah ditutup. */
function TempPasswordNotice({ result, onClose }: { result: UserWithTempPassword; onClose: () => void }) {
  return (
    <Alert kind="ok">
      <div className="gap-d flex flex-wrap items-center">
        <span>
          Sandi sementara untuk <strong>{result.user.email}</strong>:
        </span>
        <code className="rounded bg-surface px-2 font-mono select-all" aria-label="Sandi sementara">
          {result.sandi_sementara}
        </code>
        <span className="text-muted">Sampaikan secara aman. Sandi ini tidak akan ditampilkan lagi.</span>
        <Button onClick={onClose}>Tutup</Button>
      </div>
    </Alert>
  )
}

function CreateUserForm({ onCreated }: { onCreated: (r: UserWithTempPassword) => void }) {
  const qc = useQueryClient()
  const [nama, setNama] = useState('')
  const [email, setEmail] = useState('')
  const [peran, setPeran] = useState<Role>('viewer')
  const create = useMutation({
    mutationFn: () => api<UserWithTempPassword>('POST', '/admin/users', { nama, email, peran }),
    onSuccess: (r) => {
      setNama('')
      setEmail('')
      setPeran('viewer')
      void qc.invalidateQueries({ queryKey: USERS_KEY })
      onCreated(r)
    },
  })

  function submit(e: FormEvent) {
    e.preventDefault()
    create.mutate()
  }

  return (
    <form onSubmit={submit} className="gap-d grid grid-cols-1 items-end md:grid-cols-4" aria-label="Buat akun">
      <TextField label="Nama" value={nama} onChange={(e) => setNama(e.target.value)} required />
      <TextField label="Email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
      <SelectField label="Peran" value={peran} onChange={(e) => setPeran(e.target.value as Role)}>
        {ROLES.map((r) => (
          <option key={r} value={r}>
            {ROLE_LABEL[r]}
          </option>
        ))}
      </SelectField>
      <Button type="submit" variant="primary" disabled={create.isPending || !nama || !email}>
        Buat akun
      </Button>
      {create.isError && (
        <div className="md:col-span-4">
          <Alert>{errorMessage(create.error)}</Alert>
        </div>
      )}
    </form>
  )
}

function isLocked(u: AdminUser): boolean {
  return u.terkunci_hingga !== null && new Date(u.terkunci_hingga) > new Date()
}

function UserRow({
  user,
  isSelf,
  onTempPassword,
  onError,
}: {
  user: AdminUser
  isSelf: boolean
  onTempPassword: (r: UserWithTempPassword) => void
  onError: (msg: string) => void
}) {
  const qc = useQueryClient()
  const done = () => qc.invalidateQueries({ queryKey: USERS_KEY })
  const action = useMutation({
    mutationFn: ({ method, path, body }: { method: 'POST' | 'PATCH'; path: string; body?: unknown }) =>
      api<AdminUser | UserWithTempPassword>(method, `/admin/users/${user.id}${path}`, body),
    onSuccess: (r) => {
      if ('sandi_sementara' in r) onTempPassword(r)
      void done()
    },
    onError: (err) => onError(errorMessage(err)),
  })
  const busy = action.isPending

  return (
    <tr className="row-h border-b border-line">
      <td className="px-d">{user.nama}</td>
      <td className="px-d">{user.email}</td>
      <td className="px-d">
        <select
          aria-label={`Peran ${user.nama}`}
          className="rounded border border-line bg-surface px-1"
          value={user.peran}
          disabled={busy}
          onChange={(e) => action.mutate({ method: 'PATCH', path: '', body: { peran: e.target.value } })}
        >
          {ROLES.map((r) => (
            <option key={r} value={r}>
              {ROLE_LABEL[r]}
            </option>
          ))}
        </select>
      </td>
      <td className="px-d">
        {user.aktif ? 'Aktif' : 'Nonaktif'}
        {isLocked(user) && ' · Terkunci'}
        {user.wajib_ganti_sandi && ' · Wajib ganti sandi'}
      </td>
      <td className="px-d">
        <div className="flex flex-wrap gap-1 py-1">
          {user.aktif ? (
            <Button
              variant="danger"
              disabled={busy || isSelf}
              onClick={() => {
                if (window.confirm(`Nonaktifkan ${user.email}? Semua sesinya akan dicabut.`))
                  action.mutate({ method: 'POST', path: '/deactivate' })
              }}
            >
              Nonaktifkan
            </Button>
          ) : (
            <Button disabled={busy} onClick={() => action.mutate({ method: 'POST', path: '/activate' })}>
              Aktifkan
            </Button>
          )}
          <Button
            disabled={busy || isSelf}
            onClick={() => {
              if (window.confirm(`Reset sandi ${user.email}? Semua sesinya akan dicabut.`))
                action.mutate({ method: 'POST', path: '/reset-password' })
            }}
          >
            Reset sandi
          </Button>
          {isLocked(user) && (
            <Button disabled={busy} onClick={() => action.mutate({ method: 'POST', path: '/unlock' })}>
              Buka kunci
            </Button>
          )}
        </div>
      </td>
    </tr>
  )
}

export function UsersPage() {
  const me = useMe()
  const [q, setQ] = useState('')
  const users = useUsers(q)
  const [temp, setTemp] = useState<UserWithTempPassword | null>(null)
  const [rowError, setRowError] = useState<string | null>(null)

  return (
    <div className="gap-d flex flex-col">
      <h1 className="text-lg font-semibold">Pengguna</h1>
      {temp && <TempPasswordNotice result={temp} onClose={() => setTemp(null)} />}
      {rowError && <Alert>{rowError}</Alert>}
      <Panel title="Buat akun">
        <CreateUserForm onCreated={setTemp} />
      </Panel>
      <Panel
        title="Daftar pengguna"
        actions={
          <input
            aria-label="Cari pengguna"
            placeholder="Cari nama atau email"
            className="px-d rounded border border-line"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
        }
      >
        {users.isPending && <Loading />}
        {users.isError && <Alert>{errorMessage(users.error)}</Alert>}
        {users.data && (
          <table className="w-full text-left">
            <thead>
              <tr className="row-h border-b border-line text-muted">
                <th className="px-d font-medium">Nama</th>
                <th className="px-d font-medium">Email</th>
                <th className="px-d font-medium">Peran</th>
                <th className="px-d font-medium">Status</th>
                <th className="px-d font-medium">Tindakan</th>
              </tr>
            </thead>
            <tbody>
              {users.data.items.map((u) => (
                <UserRow
                  key={u.id}
                  user={u}
                  isSelf={u.id === me.data?.id}
                  onTempPassword={(r) => {
                    setRowError(null)
                    setTemp(r)
                  }}
                  onError={setRowError}
                />
              ))}
            </tbody>
          </table>
        )}
      </Panel>
    </div>
  )
}
