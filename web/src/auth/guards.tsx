import { Navigate, Outlet, useLocation } from 'react-router'

import { Alert, Loading } from '../components/ui'
import { errorMessage } from '../api/client'
import { useMe } from './session'

/** Rute yang butuh login. Pengguna yang wajib ganti sandi diarahkan ke /ganti-sandi. */
export function RequireAuth() {
  const me = useMe()
  const loc = useLocation()
  if (me.isPending) return <Loading />
  if (me.isError) return <Alert>{errorMessage(me.error)}</Alert>
  if (!me.data) return <Navigate to="/login" replace state={{ from: loc.pathname }} />
  if (me.data.wajib_ganti_sandi && loc.pathname !== '/ganti-sandi') return <Navigate to="/ganti-sandi" replace />
  return <Outlet />
}

/** Rute khusus Admin. Pemeriksaan sebenarnya tetap di server. */
export function RequireAdmin() {
  const me = useMe()
  if (me.data?.peran !== 'admin') return <Navigate to="/" replace />
  return <Outlet />
}
