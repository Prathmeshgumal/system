import { useEffect, useState } from 'react'
import { fetchStatus, type StatusResponse, type StatusResult } from './api'
import './StatusPage.css'

const POLL_MS = 5000

export function StatusPage() {
  const [result, setResult] = useState<StatusResult | null>(null)
  const [checkedAt, setCheckedAt] = useState<number | null>(null)
  const now = useNow()

  // Check now, then every 5 seconds. Abort the in-flight request on unmount.
  useEffect(() => {
    const controller = new AbortController()
    const check = async () => {
      const r = await fetchStatus(controller.signal)
      if (controller.signal.aborted) return
      setResult(r)
      setCheckedAt(Date.now())
    }
    check()
    const id = setInterval(check, POLL_MS)
    return () => {
      clearInterval(id)
      controller.abort()
    }
  }, [])

  return (
    <main className="status">
      <h1>Relay — System Status</h1>
      {result === null ? (
        <p className="muted">Checking…</p>
      ) : (
        <dl>
          <dt>API</dt>
          <dd>
            {result.reachable ? <Dot ok label="healthy" /> : <Dot ok={false} label="unreachable" />}
          </dd>

          <dt>Postgres</dt>
          <dd>
            {!result.reachable ? (
              <span className="muted">unknown</span>
            ) : result.data.postgres.status === 'ok' ? (
              <>
                <Dot ok label="healthy" />{' '}
                <span className="muted">({result.data.postgres.latency_ms} ms)</span>
              </>
            ) : (
              <Dot ok={false} label="down" />
            )}
          </dd>

          {result.reachable && (
            <>
              <dt>Pool</dt>
              <dd>
                <PoolStats pool={result.data.db_pool} />
              </dd>

              <dt>Version</dt>
              <dd>
                {result.data.version} <span className="muted">· up {result.data.uptime_s}s</span>
              </dd>
            </>
          )}

          <dt>Last check</dt>
          <dd className="muted">
            {checkedAt === null ? '—' : `${Math.max(0, Math.round((now - checkedAt) / 1000))}s ago`}
          </dd>
        </dl>
      )}
    </main>
  )
}

// Live connection-pool numbers (knowledge-base/notes/phase-0-notes.md, note 11).
function PoolStats({ pool }: { pool: StatusResponse['db_pool'] }) {
  return (
    <>
      {pool.in_use} in use · {pool.idle} idle{' '}
      <span className="muted">· max {pool.max}</span>
      {pool.waited > 0 && <span className="down"> · {pool.waited} waited</span>}
    </>
  )
}

function Dot({ ok, label }: { ok: boolean; label: string }) {
  return (
    <span className={ok ? 'ok' : 'down'}>
      <span aria-hidden="true">●</span> {label}
    </span>
  )
}

// Re-renders every second so "Last check Ns ago" stays current.
function useNow() {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(id)
  }, [])
  return now
}
