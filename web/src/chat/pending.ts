// Messages I've sent that the server hasn't confirmed yet. They show at the
// bottom of the chat right away ("optimistic" UI) and disappear when the real
// message — matched by client_id — arrives from the POST response or the WebSocket.
import { useSyncExternalStore } from 'react'

export type Pending = {
  client_id: string
  chat_id: number
  body: string
  status: 'sending' | 'failed'
  reply_to_id?: number
}

let pending: Pending[] = []
const listeners = new Set<() => void>()

function emit(next: Pending[]) {
  pending = next
  listeners.forEach((l) => l())
}

export const pendingStore = {
  add: (p: Pending) => emit([...pending, p]),
  setStatus: (clientId: string, status: Pending['status']) =>
    emit(pending.map((p) => (p.client_id === clientId ? { ...p, status } : p))),
  remove: (clientId: string) => {
    if (pending.some((p) => p.client_id === clientId)) emit(pending.filter((p) => p.client_id !== clientId))
  },
  clear: () => emit([]),
}

function subscribe(l: () => void) {
  listeners.add(l)
  return () => listeners.delete(l)
}

export function usePending(chatId: number): Pending[] {
  const all = useSyncExternalStore(subscribe, () => pending)
  return all.filter((p) => p.chat_id === chatId)
}
