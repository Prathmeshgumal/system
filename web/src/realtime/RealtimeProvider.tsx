import { useQueryClient } from '@tanstack/react-query'
import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { useNavigate } from 'react-router'
import { useMe } from '../auth/AuthProvider'
import { api } from '../lib/api'
import { showNotification } from '../lib/notify'
import { catchUp, handleEvent } from './events'
import { RealtimeSocket, type SocketStatus } from './socket'

type Realtime = {
  status: SocketStatus
  /** Sends a client frame (only typing, today). Dropped silently when offline. */
  send: (frame: { type: 'typing'; chat_id: number }) => void
}

const RealtimeContext = createContext<Realtime>({ status: 'connecting', send: () => {} })

/** Keeps one WebSocket open while logged in, and feeds its events into the query cache. */
export function RealtimeProvider({ children }: { children: ReactNode }) {
  const qc = useQueryClient()
  const me = useMe()
  const navigate = useNavigate()
  const navigateRef = useRef(navigate)
  useEffect(() => {
    navigateRef.current = navigate
  }, [navigate])
  const socketRef = useRef<RealtimeSocket | null>(null)
  const [status, setStatus] = useState<SocketStatus>('connecting')

  useEffect(() => {
    const socket = new RealtimeSocket({
      getTicket: () => api<{ ticket: string }>('/api/ws/ticket', { method: 'POST' }).then((r) => r.ticket),
      // Same host as the page: Vite (dev) or Caddy (full mode) forwards /ws to Go.
      url: (t) => `${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/ws?ticket=${encodeURIComponent(t)}`,
      onEvent: (e) =>
        handleEvent(qc, e, me.id, {
          notify: (m) =>
            showNotification(m.sender?.display_name ?? 'New message', m.body, `chat-${m.chat_id}`, () =>
              navigateRef.current(`/chats/${m.chat_id}`),
            ),
        }),
      onOpen: () => void catchUp(qc),
      onStatus: setStatus,
    })
    socketRef.current = socket
    socket.start()
    return () => socket.stop()
  }, [qc, me.id])

  const value: Realtime = { status, send: (frame) => void socketRef.current?.send(frame) }
  return <RealtimeContext.Provider value={value}>{children}</RealtimeContext.Provider>
}

export function useSocketStatus() {
  return useContext(RealtimeContext).status
}

export function useRealtime() {
  return useContext(RealtimeContext)
}
