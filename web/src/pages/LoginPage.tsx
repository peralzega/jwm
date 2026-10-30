import { useState, type FormEvent } from 'react'
import { Navigate, useLocation, useNavigate } from 'react-router'

import { errorMessage } from '../api/client'
import { useLogin, useMe } from '../auth/session'
import { Alert, Button, TextField } from '../components/ui'

export function LoginPage() {
  const me = useMe()
  const login = useLogin()
  const navigate = useNavigate()
  const loc = useLocation()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')

  if (me.data) return <Navigate to="/" replace />

  const from = (loc.state as { from?: string } | null)?.from ?? '/'

  function submit(e: FormEvent) {
    e.preventDefault()
    login.mutate(
      { email, password },
      {
        onSuccess: (r) => navigate(r.user.wajib_ganti_sandi ? '/ganti-sandi' : from, { replace: true }),
        onSettled: () => setPassword(''),
      },
    )
  }

  return (
    <main className="mx-auto mt-24 w-full max-w-sm">
      <form onSubmit={submit} className="gap-d flex flex-col rounded border border-line bg-surface p-6" noValidate>
        <h1 className="text-lg font-semibold">Masuk ke JWM</h1>
        {login.isError && <Alert>{errorMessage(login.error)}</Alert>}
        <TextField
          label="Email"
          type="email"
          autoComplete="username"
          required
          value={email}
          onChange={(e) => setEmail(e.target.value)}
        />
        <TextField
          label="Kata sandi"
          type="password"
          autoComplete="current-password"
          required
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
        <Button type="submit" variant="primary" disabled={login.isPending || !email || !password}>
          {login.isPending ? 'Memproses…' : 'Masuk'}
        </Button>
        <p className="text-muted">Lupa kata sandi? Hubungi Admin untuk reset.</p>
      </form>
    </main>
  )
}
