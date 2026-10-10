import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError, refreshSession, setAccessToken, setOnSessionLost } from './api'

function json(status: number, body: unknown) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

const authBody = { access_token: 'new-token', expires_at: '', user: { id: 1 } }

describe('api client', () => {
  let fetchMock: ReturnType<typeof vi.fn>

  beforeEach(() => {
    fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    setAccessToken('old-token')
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    setOnSessionLost(() => {})
  })

  it('sends the access token as a Bearer header', async () => {
    fetchMock.mockResolvedValueOnce(json(200, { ok: true }))
    await api('/api/me')
    const headers = fetchMock.mock.calls[0][1].headers as Headers
    expect(headers.get('Authorization')).toBe('Bearer old-token')
  })

  it('on 401 refreshes once, then retries with the new token', async () => {
    fetchMock
      .mockResolvedValueOnce(json(401, { error: { code: 'unauthorized', message: 'expired' } }))
      .mockResolvedValueOnce(json(200, authBody)) // POST /api/auth/refresh
      .mockResolvedValueOnce(json(200, { id: 1 })) // the retry

    const me = await api<{ id: number }>('/api/me')

    expect(me.id).toBe(1)
    expect(fetchMock.mock.calls[1][0]).toBe('/api/auth/refresh')
    expect((fetchMock.mock.calls[2][1].headers as Headers).get('Authorization')).toBe('Bearer new-token')
  })

  it('many parallel 401s share ONE refresh', async () => {
    let refreshes = 0
    fetchMock.mockImplementation(async (url: string, init: RequestInit) => {
      if (url === '/api/auth/refresh') {
        refreshes++
        await new Promise((r) => setTimeout(r, 10))
        return json(200, authBody)
      }
      const auth = (init.headers as Headers).get('Authorization')
      return auth === 'Bearer new-token' ? json(200, {}) : json(401, { error: { code: 'unauthorized', message: '' } })
    })

    await Promise.all([api('/api/a'), api('/api/b'), api('/api/c')])
    expect(refreshes).toBe(1)
  })

  it('when refresh fails, reports the session lost and throws', async () => {
    const lost = vi.fn()
    setOnSessionLost(lost)
    fetchMock
      .mockResolvedValueOnce(json(401, { error: { code: 'unauthorized', message: '' } }))
      .mockResolvedValueOnce(json(401, { error: { code: 'unauthorized', message: '' } }))

    await expect(api('/api/me')).rejects.toMatchObject({ status: 401 })
    expect(lost).toHaveBeenCalledOnce()
  })

  it('turns error bodies into ApiError with code and fields', async () => {
    fetchMock.mockResolvedValueOnce(
      json(400, { error: { code: 'invalid_input', message: 'bad', fields: { email: 'nope' } } }),
    )
    const err = (await api('/api/x', { method: 'POST', body: {} }).catch((e: unknown) => e)) as ApiError
    expect(err).toBeInstanceOf(ApiError)
    expect(err.code).toBe('invalid_input')
    expect(err.fields).toEqual({ email: 'nope' })
  })

  it('a network failure becomes ApiError with status 0', async () => {
    fetchMock.mockRejectedValueOnce(new TypeError('Failed to fetch'))
    await expect(api('/api/me')).rejects.toMatchObject({ status: 0, code: 'network' })
  })

  it('refreshSession stores the new access token', async () => {
    fetchMock.mockResolvedValueOnce(json(200, authBody)).mockResolvedValueOnce(json(200, {}))
    await refreshSession()
    await api('/api/me')
    expect((fetchMock.mock.calls[1][1].headers as Headers).get('Authorization')).toBe('Bearer new-token')
  })
})
