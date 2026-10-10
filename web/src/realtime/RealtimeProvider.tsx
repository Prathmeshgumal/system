import { useQueryClient } from '@tanstack/react-query'
import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'
import { useMe } from '../auth/AuthProvider'
import { api } from '../lib/api'
import { catchUp, handleEvent } from './events'
import { RealtimeSocket, type SocketStatus } from './socket'

const StatusContext = createContext<SocketStatus>('connecting')

/** Keeps one WebSocket open while logged in, and feeds its events into the query cache. */
export function RealtimeProvider({ children }: { children: ReactNode }) {
  const qc = useQueryClient()
  const me = useMe()
  const [status, setStatus] = useState<SocketStatus>('connecting')

  useEffect(() => {
    const socket = new RealtimeSocket({
      getTicket: () => api<{ ticket: string }>('/api/ws/ticket', { method: 'POST' }).then((r) => r.ticket),
      // Same host as the page: Vite (dev) or Caddy (full mode) forwards /ws to Go.
      url: (t) => `${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/ws?ticket=${encodeURIComponent(t)}`,
      onEvent: (e) => handleEvent(qc, e, me.id),
      onOpen: () => void catchUp(qc),
      onStatus: setStatus,
    })
    socket.start()
    return () => socket.stop()
  }, [qc, me.id])

  return <StatusContext.Provider value={status}>{children}</StatusContext.Provider>
}

export function useSocketStatus() {
  return useContext(StatusContext)
}
