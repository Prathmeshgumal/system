import { act, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { StatusPage } from './StatusPage'

const healthyBody = {
  api: 'ok',
  postgres: { status: 'ok', latency_ms: 2 },
  db_pool: { in_use: 1, idle: 2, total: 3, max: 10, waited: 0 },
  version: 'abc123',
  uptime_s: 42,
}

const dbDownBody = {
  api: 'ok',
  postgres: { status: 'down', latency_ms: 0.4 },
  db_pool: { in_use: 0, idle: 0, total: 0, max: 10, waited: 0 },
  version: 'abc123',
  uptime_s: 42,
}

function jsonResponse(status: number, body: unknown) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

// Reads the value next to a label in the status list, e.g. valueOf('API') → "healthy".
async function valueOf(label: string) {
  const term = await screen.findByText(label, { selector: 'dt' })
  return term.nextElementSibling?.textContent ?? ''
}

let fetchMock: ReturnType<typeof vi.fn>

beforeEach(() => {
  fetchMock = vi.fn()
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

describe('StatusPage', () => {
  it('calls /api/status (through the proxy, same origin)', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, healthyBody))

    render(<StatusPage />)

    expect(await valueOf('API')).toContain('healthy')
    expect(fetchMock).toHaveBeenCalledWith('/api/status', expect.anything())
  })

  it('shows everything healthy when the API and DB are up', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, healthyBody))

    render(<StatusPage />)

    expect(await valueOf('API')).toContain('healthy')
    expect(await valueOf('Postgres')).toContain('healthy')
    expect(await valueOf('Postgres')).toContain('2 ms')
    expect(await valueOf('Version')).toContain('abc123')
    expect(await valueOf('Version')).toContain('up 42s')
  })

  it('shows the connection pool numbers', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, healthyBody))

    render(<StatusPage />)

    const pool = await valueOf('Pool')
    expect(pool).toContain('1 in use')
    expect(pool).toContain('2 idle')
    expect(pool).toContain('max 10')
  })

  it('mentions waits only when requests had to wait for a connection', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse(200, { ...healthyBody, db_pool: { ...healthyBody.db_pool, waited: 5 } }),
    )

    render(<StatusPage />)

    expect(await valueOf('Pool')).toContain('5 waited')
  })

  it('shows the API healthy but Postgres down when the API returns 503', async () => {
    fetchMock.mockResolvedValue(jsonResponse(503, dbDownBody))

    render(<StatusPage />)

    expect(await valueOf('API')).toContain('healthy')
    expect(await valueOf('Postgres')).toContain('down')
  })

  it('shows the API unreachable when the request fails', async () => {
    fetchMock.mockRejectedValue(new TypeError('Failed to fetch'))

    render(<StatusPage />)

    expect(await valueOf('API')).toContain('unreachable')
    expect(await valueOf('Postgres')).toContain('unknown')
  })

  it('shows the API unreachable when the proxy answers 502 with a non-JSON page', async () => {
    fetchMock.mockResolvedValue(new Response('<html>Bad Gateway</html>', { status: 502 }))

    render(<StatusPage />)

    expect(await valueOf('API')).toContain('unreachable')
  })

  it('checks again every 5 seconds and picks up recovery', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    fetchMock
      .mockResolvedValueOnce(jsonResponse(503, dbDownBody))
      .mockResolvedValue(jsonResponse(200, healthyBody))

    render(<StatusPage />)
    expect(await valueOf('Postgres')).toContain('down')

    await act(() => vi.advanceTimersByTimeAsync(5000))

    expect(await valueOf('Postgres')).toContain('healthy')
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })
})
