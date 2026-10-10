import { useQueryClient } from '@tanstack/react-query'
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { pendingStore } from '../chat/pending'
import { api, refreshSession, setAccessToken, setOnSessionLost } from '../lib/api'
import { typingStore } from '../realtime/typing'
import type { AuthResponse, User } from '../lib/types'

type AuthState = { status: 'loading' } | { status: 'out' } | { status: 'in'; user: User }

type SignupInput = { email: string; username: string; display_name: string; password: string }

type AuthContextValue = {
  state: AuthState
  login: (email: string, password: string) => Promise<void>
  signup: (input: SignupInput) => Promise<void>
  logout: () => Promise<void>
  setUser: (u: User) => void
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const qc = useQueryClient()
  const [state, setState] = useState<AuthState>({ status: 'loading' })

  const signedOut = useCallback(() => {
    setAccessToken(null)
    qc.clear() // never show the previous user's chats to the next one
    pendingStore.clear()
    typingStore.reset()
    setState({ status: 'out' })
  }, [qc])

  // On load: the access token was only in memory, so it's gone. If the refresh
  // cookie is still valid, this gets a new one and we're logged in again.
  useEffect(() => {
    setOnSessionLost(signedOut)
    refreshSession()
      .then((a) => setState({ status: 'in', user: a.user }))
      .catch(() => setState({ status: 'out' }))
  }, [signedOut])

  const signedIn = useCallback((a: AuthResponse) => {
    setAccessToken(a.access_token)
    setState({ status: 'in', user: a.user })
  }, [])

  const value = useMemo<AuthContextValue>(
    () => ({
      state,
      login: async (email, password) =>
        signedIn(await api<AuthResponse>('/api/auth/login', { method: 'POST', body: { email, password } })),
      signup: async (input) => signedIn(await api<AuthResponse>('/api/auth/signup', { method: 'POST', body: input })),
      logout: async () => {
        try {
          await api('/api/auth/logout', { method: 'POST' })
        } finally {
          signedOut()
        }
      },
      setUser: (user) => setState({ status: 'in', user }),
    }),
    [state, signedIn, signedOut],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used inside <AuthProvider>')
  return ctx
}

/** The logged-in user. Only for components rendered behind <RequireAuth>. */
export function useMe(): User {
  const { state } = useAuth()
  if (state.status !== 'in') throw new Error('useMe used while logged out')
  return state.user
}
