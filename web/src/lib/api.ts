// The one way the app talks to the backend over HTTP.
//
// - The access token lives only in this module's memory (never localStorage):
//   an XSS bug can't steal what isn't stored. It's lost on reload, which is
//   why the app calls refreshSession() on startup.
// - The refresh token is an httpOnly cookie the browser sends to /api/auth/*
//   by itself; JavaScript never sees it. See knowledge-base/adr/0001-auth-tokens.md.
import type { AuthResponse } from './types'

export class ApiError extends Error {
  status: number
  code: string
  fields: Record<string, string>

  constructor(status: number, code: string, message: string, fields: Record<string, string> = {}) {
    super(message)
    this.status = status
    this.code = code
    this.fields = fields
  }
}

let accessToken: string | null = null
let onSessionLost: () => void = () => {}

export function setAccessToken(token: string | null) {
  accessToken = token
}

/** Called when the session can't be renewed (logged out elsewhere, expired, revoked). */
export function setOnSessionLost(fn: () => void) {
  onSessionLost = fn
}

type Options = {
  method?: string
  body?: unknown
  signal?: AbortSignal
}

/** Calls the API with the access token. On a 401 it renews the token once and retries. */
export async function api<T = unknown>(path: string, opts: Options = {}): Promise<T> {
  const res = await send(path, opts)
  if (res.status !== 401 || path.startsWith('/api/auth/')) {
    return parse<T>(res)
  }
  try {
    await refreshSession()
  } catch (err) {
    onSessionLost()
    throw err
  }
  return parse<T>(await send(path, opts))
}

function send(path: string, opts: Options): Promise<Response> {
  const headers = new Headers({ Accept: 'application/json' })
  if (opts.body !== undefined) headers.set('Content-Type', 'application/json')
  if (accessToken) headers.set('Authorization', `Bearer ${accessToken}`)
  return fetch(path, {
    method: opts.method ?? 'GET',
    headers,
    body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
    signal: opts.signal,
  }).catch((err: unknown) => {
    if (err instanceof DOMException && err.name === 'AbortError') throw err
    throw new ApiError(0, 'network', "Can't reach the server")
  })
}

async function parse<T>(res: Response): Promise<T> {
  const isJSON = res.headers.get('Content-Type')?.includes('application/json')
  const data = isJSON ? await res.json() : undefined
  if (!res.ok) {
    const e = data?.error
    throw new ApiError(res.status, e?.code ?? 'http_error', e?.message ?? `Request failed (${res.status})`, e?.fields)
  }
  return data as T
}

// ---- refreshing ----

let inflight: Promise<AuthResponse> | null = null

/**
 * Swaps the refresh cookie for a new access token (and a new cookie).
 *
 * Two guards against using the same refresh token twice — which the server
 * treats as theft and answers by revoking the session:
 *  1. Within this tab, parallel callers share one in-flight request.
 *  2. Across tabs (they share the cookie), the Web Locks API lets one tab
 *     refresh at a time; the next one then sends the already-rotated cookie.
 */
export function refreshSession(): Promise<AuthResponse> {
  inflight ??= withCrossTabLock(async () => {
    const auth = await parse<AuthResponse>(await send('/api/auth/refresh', { method: 'POST' }))
    setAccessToken(auth.access_token)
    return auth
  }).finally(() => {
    inflight = null
  })
  return inflight
}

function withCrossTabLock<T>(fn: () => Promise<T>): Promise<T> {
  if (typeof navigator !== 'undefined' && navigator.locks) {
    return navigator.locks.request('relay-refresh', fn)
  }
  return fn() // no Web Locks (old browser, or tests): single-tab protection only
}
