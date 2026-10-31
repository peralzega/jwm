import type { RouteObject } from 'react-router'

import { RequireAdmin, RequireAuth } from './auth/guards'
import { Layout } from './components/Layout'
import { CalendarPage } from './pages/admin/CalendarPage'
import { UsersPage } from './pages/admin/UsersPage'
import { BoardPage } from './pages/BoardPage'
import { ChangePasswordPage } from './pages/ChangePasswordPage'
import { LoginPage } from './pages/LoginPage'
import { NotFoundPage } from './pages/NotFoundPage'
import { PortfolioPage } from './pages/PortfolioPage'
import { ProjectPage } from './pages/ProjectPage'
import { ProjectsPage } from './pages/ProjectsPage'

export const routes: RouteObject[] = [
  { path: '/login', element: <LoginPage /> },
  {
    element: <RequireAuth />,
    children: [
      { path: '/ganti-sandi', element: <ChangePasswordPage /> },
      {
        element: <Layout />,
        children: [
          { path: '/', element: <ProjectsPage /> },
          { path: '/portofolio', element: <PortfolioPage /> },
          { path: '/proyek/:projectId', element: <ProjectPage /> },
          { path: '/board/:boardId', element: <BoardPage /> },
          {
            element: <RequireAdmin />,
            children: [
              { path: '/admin/pengguna', element: <UsersPage /> },
              { path: '/admin/kalender', element: <CalendarPage /> },
            ],
          },
          { path: '*', element: <NotFoundPage /> },
        ],
      },
    ],
  },
]
