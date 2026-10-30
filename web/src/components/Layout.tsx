import { NavLink, Outlet, useNavigate } from 'react-router'

import { ROLE_LABEL } from '../api/types'
import { useLogout, useMe } from '../auth/session'
import { Button } from './ui'

const NAV = 'px-d row-h inline-flex items-center rounded hover:bg-canvas'
const NAV_ACTIVE = 'bg-canvas font-semibold'

export function Layout() {
  const me = useMe()
  const logout = useLogout()
  const navigate = useNavigate()
  const user = me.data

  return (
    <div className="min-h-screen">
      <header className="px-d gap-d flex items-center border-b border-line bg-surface">
        <span className="font-semibold text-accent">JWM</span>
        <nav aria-label="Navigasi utama" className="flex gap-1">
          <NavLink to="/" end className={({ isActive }) => `${NAV} ${isActive ? NAV_ACTIVE : ''}`}>
            Proyek
          </NavLink>
          {user?.peran === 'admin' && (
            <NavLink to="/admin/pengguna" className={({ isActive }) => `${NAV} ${isActive ? NAV_ACTIVE : ''}`}>
              Pengguna
            </NavLink>
          )}
        </nav>
        <div className="gap-d ml-auto flex items-center py-1">
          {user && (
            <span className="text-muted">
              {user.nama} · {ROLE_LABEL[user.peran]}
            </span>
          )}
          <NavLink to="/ganti-sandi" className={NAV}>
            Ganti sandi
          </NavLink>
          <Button onClick={() => logout.mutate(undefined, { onSettled: () => navigate('/login', { replace: true }) })}>
            Keluar
          </Button>
        </div>
      </header>
      <main className="p-4">
        <Outlet />
      </main>
    </div>
  )
}
