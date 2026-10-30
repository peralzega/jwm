import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render } from '@testing-library/react'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { vi } from 'vitest'

import type { SessionUser } from '../api/types'
import { routes } from '../routes'

export interface Call {
  method: string
  path: string
  body: unknown
  headers: Record<string, string>
}

type Reply = { status: number; body?: unknown }
type Handler = (call: Call) => Reply | Promise<Reply>

/**
 * Mengganti fetch dengan tabel rute "METHOD /path" (path tanpa /api/v1 dan
 * tanpa query string). Rute yang tidak terdaftar dijawab 404.
 */
export function mockApi(handlers: Record<string, Handler | Reply>) {
  const calls: Call[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: string, init?: RequestInit) => {
      const url = new URL(input, 'http://jwm.test')
      const path = url.pathname.replace(/^\/api\/v1/, '')
      const method = init?.method ?? 'GET'
      const call: Call = {
        method,
        path,
        body: init?.body ? JSON.parse(String(init.body)) : undefined,
        headers: (init?.headers ?? {}) as Record<string, string>,
      }
      calls.push(call)
      const h = handlers[`${method} ${path}`]
      const reply = h === undefined ? { status: 404, body: { error: 'not_found', message: 'tidak ada' } } : typeof h === 'function' ? await h(call) : h
      return new Response(reply.body === undefined ? null : JSON.stringify(reply.body), { status: reply.status })
    }),
  )
  return calls
}

export function user(over: Partial<SessionUser> = {}): SessionUser {
  return { id: 'u-1', nama: 'Budi', email: 'budi@jiep.co.id', peran: 'member', wajib_ganti_sandi: false, kepadatan: 'compact', ...over }
}

export function meReply(u: SessionUser | null): Reply {
  return u ? { status: 200, body: { user: u, csrf_token: 'csrf-me' } } : { status: 401, body: { error: 'unauthenticated' } }
}

export function renderApp(path: string) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(routes, { initialEntries: [path] })
  render(
    <QueryClientProvider client={qc}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return router
}
