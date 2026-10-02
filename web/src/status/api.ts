// Shape of GET /api/status — mirrors StatusResponse in backend/internal/health/handler.go.
export type StatusResponse = {
  api: 'ok'
  postgres: { status: 'ok' | 'down'; latency_ms: number }
  version: string
  uptime_s: number
}

export type StatusResult =
  | { reachable: true; data: StatusResponse }
  | { reachable: false } // network error, or something other than our API answered (e.g. proxy 502)

// fetchStatus never throws: every failure becomes { reachable: false }.
// 200 and 503 both carry a JSON body from our API, so both count as reachable.
export async function fetchStatus(signal?: AbortSignal): Promise<StatusResult> {
  try {
    const res = await fetch('/api/status', { signal, headers: { Accept: 'application/json' } })
    if (!res.headers.get('Content-Type')?.includes('application/json')) {
      return { reachable: false }
    }
    const data = (await res.json()) as StatusResponse
    return { reachable: true, data }
  } catch {
    return { reachable: false }
  }
}
