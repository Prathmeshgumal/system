import { Navigate, Outlet, useLocation } from 'react-router'
import { useAuth } from './AuthProvider'

export function RequireAuth() {
  const { state } = useAuth()
  const location = useLocation()
  if (state.status === 'loading') {
    return <div className="flex h-dvh items-center justify-center text-zinc-500">Loading…</div>
  }
  if (state.status === 'out') {
    return <Navigate to="/login" replace state={{ from: location.pathname }} />
  }
  return <Outlet />
}
