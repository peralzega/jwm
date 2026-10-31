import { NavLink, Outlet, useNavigate } from 'react-router'

import { ROLE_LABEL } from '../api/types'
import { useLogout, useMe } from '../auth/session'
import { DensityToggle } from './DensityToggle'
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
      <header className="px-d gap-d flex items-center border-b border-line bg-surface print:hidden">
        <span className="font-semibold text-primary">JWM</span>
        <nav aria-label="Navigasi utama" className="flex gap-(--space-sm)">
          <NavLink to="/" end className={({ isActive }) => `${NAV} ${isActive ? NAV_ACTIVE : ''}`}>
            Proyek
          </NavLink>
          <NavLink to="/portofolio" className={({ isActive }) => `${NAV} ${isActive ? NAV_ACTIVE : ''}`}>
            Portofolio
          </NavLink>
          <NavLink to="/dasbor" className={({ isActive }) => `${NAV} ${isActive ? NAV_ACTIVE : ''}`}>
            Dasbor
          </NavLink>
          {user?.peran === 'admin' && (
            <>
              <NavLink to="/admin/pengguna" className={({ isActive }) => `${NAV} ${isActive ? NAV_ACTIVE : ''}`}>
                Pengguna
              </NavLink>
              <NavLink to="/admin/kalender" className={({ isActive }) => `${NAV} ${isActive ? NAV_ACTIVE : ''}`}>
                Kalender kerja
              </NavLink>
            </>
          )}
        </nav>
        <div className="gap-d ml-auto flex items-center py-(--space-sm)">
          {user && (
            <span className="text-muted-foreground">
              {user.nama} · {ROLE_LABEL[user.peran]}
            </span>
          )}
          <DensityToggle />
          <NavLink to="/ganti-sandi" className={NAV}>
            Ganti sandi
          </NavLink>
          <Button onClick={() => logout.mutate(undefined, { onSettled: () => navigate('/login', { replace: true }) })}>
            Keluar
          </Button>
        </div>
      </header>
      <main className="p-(--page-pad)">
        <Outlet />
      </main>
    </div>
  )
}
