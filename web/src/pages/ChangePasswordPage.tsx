import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router'

import { errorMessage } from '../api/client'
import { useChangePassword, useMe } from '../auth/session'
import { Alert, Button, TextField } from '../components/ui'

const MIN_LEN = 12

/** Validasi ringan di klien; kebijakan lengkap (daftar sandi umum) tetap di server. */
export function validateNewPassword(lama: string, baru: string, ulang: string): string | null {
  if ([...baru].length < MIN_LEN) return `Kata sandi baru minimal ${MIN_LEN} karakter.`
  if (baru !== ulang) return 'Konfirmasi kata sandi tidak sama.'
  if (baru === lama) return 'Kata sandi baru harus berbeda dari yang lama.'
  return null
}

export function ChangePasswordPage() {
  const me = useMe()
  const change = useChangePassword()
  const navigate = useNavigate()
  const [lama, setLama] = useState('')
  const [baru, setBaru] = useState('')
  const [ulang, setUlang] = useState('')
  const [localError, setLocalError] = useState<string | null>(null)

  const wajib = me.data?.wajib_ganti_sandi ?? false

  function submit(e: FormEvent) {
    e.preventDefault()
    const problem = validateNewPassword(lama, baru, ulang)
    setLocalError(problem)
    if (problem) return
    change.mutate(
      { sandi_lama: lama, sandi_baru: baru },
      {
        onSuccess: () => navigate('/', { replace: true }),
        onSettled: () => {
          setLama('')
          setBaru('')
          setUlang('')
        },
      },
    )
  }

  return (
    <main className="mx-auto mt-16 w-full max-w-sm">
      <form onSubmit={submit} className="gap-d flex flex-col rounded border border-line bg-surface p-6" noValidate>
        <h1 className="text-lg font-semibold">Ganti kata sandi</h1>
        {wajib && <Alert kind="ok">Anda wajib mengganti kata sandi sementara sebelum melanjutkan.</Alert>}
        {(localError ?? change.isError) && <Alert>{localError ?? errorMessage(change.error)}</Alert>}
        <TextField
          label="Kata sandi saat ini"
          type="password"
          autoComplete="current-password"
          value={lama}
          onChange={(e) => setLama(e.target.value)}
        />
        <TextField
          label="Kata sandi baru"
          type="password"
          autoComplete="new-password"
          hint={`Minimal ${MIN_LEN} karakter. Hindari sandi umum.`}
          value={baru}
          onChange={(e) => setBaru(e.target.value)}
        />
        <TextField
          label="Ulangi kata sandi baru"
          type="password"
          autoComplete="new-password"
          value={ulang}
          onChange={(e) => setUlang(e.target.value)}
        />
        <Button type="submit" variant="primary" disabled={change.isPending || !lama || !baru}>
          {change.isPending ? 'Menyimpan…' : 'Simpan'}
        </Button>
      </form>
    </main>
  )
}
