// Who is typing where. Typing events are temporary: each one means "typing
// for the next few seconds", so every entry removes itself after
// TYPING_TTL_MS; nothing is ever stored on the server.
import { useSyncExternalStore } from 'react'

export const TYPING_TTL_MS = 6000

type State = ReadonlyMap<number, readonly number[]> // chatId → user ids typing

let state: State = new Map()
const timers = new Map<string, ReturnType<typeof setTimeout>>()
const listeners = new Set<() => void>()

function set(chatId: number, users: number[]) {
  const next = new Map(state)
  next.set(chatId, users)
  state = next
  listeners.forEach((l) => l())
}

export const typingStore = {
  /** userId is typing in chatId — for the next TYPING_TTL_MS, or until cleared. */
  add(chatId: number, userId: number) {
    const key = `${chatId}:${userId}`
    clearTimeout(timers.get(key))
    timers.set(
      key,
      setTimeout(() => typingStore.clear(chatId, userId), TYPING_TTL_MS),
    )
    const users = state.get(chatId) ?? []
    if (!users.includes(userId)) set(chatId, [...users, userId])
  },
  /** userId stopped (their message arrived, or the entry expired). */
  clear(chatId: number, userId: number) {
    const key = `${chatId}:${userId}`
    clearTimeout(timers.get(key))
    timers.delete(key)
    const users = state.get(chatId) ?? []
    if (users.includes(userId)) set(chatId, users.filter((u) => u !== userId))
  },
  get: (chatId: number): readonly number[] => state.get(chatId) ?? [],
  /** Forget everything (logout, tests). */
  reset() {
    timers.forEach(clearTimeout)
    timers.clear()
    state = new Map()
    listeners.forEach((l) => l())
  },
}

function subscribe(l: () => void) {
  listeners.add(l)
  return () => listeners.delete(l)
}

const none: readonly number[] = []

/** User ids typing in chatId right now. */
export function useTypers(chatId: number): readonly number[] {
  return useSyncExternalStore(subscribe, () => state.get(chatId) ?? none)
}
