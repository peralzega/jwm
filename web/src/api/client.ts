// Klien HTTP untuk /api/v1. Cookie sesi dikirim otomatis (same-origin);
// token CSRF disimpan di memori dan dikirim pada metode pengubah data.

let csrfToken = ''

export function setCsrfToken(token: string): void {
  csrfToken = token
}

export class ApiError extends Error {
  readonly status: number
  readonly code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

type Method = 'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE'

interface ErrorBody {
  error?: string
  message?: string
}

const FALLBACK_MESSAGE = 'Terjadi kesalahan. Coba lagi.'

export async function api<T>(method: Method, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' }
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  if (method !== 'GET' && csrfToken) headers['X-CSRF-Token'] = csrfToken

  let res: Response
  try {
    res = await fetch(`/api/v1${path}`, {
      method,
      headers,
      credentials: 'same-origin',
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  } catch {
    throw new ApiError(0, 'network', 'Tidak dapat terhubung ke server.')
  }

  if (res.status === 204) return undefined as T
  const text = await res.text()
  const data: unknown = text ? safeParse(text) : undefined

  if (!res.ok) {
    const e = (data ?? {}) as ErrorBody
    throw new ApiError(res.status, e.error ?? 'unknown', e.message ?? FALLBACK_MESSAGE)
  }
  const token = (data as { csrf_token?: unknown } | undefined)?.csrf_token
  if (typeof token === 'string') setCsrfToken(token)
  return data as T
}

function safeParse(text: string): unknown {
  try {
    return JSON.parse(text)
  } catch {
    return undefined
  }
}

export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) return err.message
  return FALLBACK_MESSAGE
}
