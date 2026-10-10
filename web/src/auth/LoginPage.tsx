import { useState, type FormEvent } from 'react'
import { Link, Navigate, useLocation } from 'react-router'
import { ApiError } from '../lib/api'
import { Field } from '../ui/Field'
import { primaryButton } from '../ui/buttons'
import { AuthLayout } from './AuthLayout'
import { useAuth } from './AuthProvider'

export function LoginPage() {
  const { state, login } = useAuth()
  const location = useLocation()
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  if (state.status === 'in') {
    const from = (location.state as { from?: string } | null)?.from ?? '/'
    return <Navigate to={from} replace />
  }

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    const form = new FormData(e.currentTarget)
    setBusy(true)
    setError('')
    try {
      await login(String(form.get('email')), String(form.get('password')))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Something went wrong')
    } finally {
      setBusy(false)
    }
  }

  return (
    <AuthLayout title="Log in">
      <form onSubmit={submit} className="space-y-4">
        <Field label="Email" name="email" type="email" autoComplete="email" required />
        <Field label="Password" name="password" type="password" autoComplete="current-password" required />
        {error && (
          <p role="alert" className="text-sm text-rose-600">
            {error}
          </p>
        )}
        <button type="submit" disabled={busy} className={`${primaryButton} w-full`}>
          {busy ? 'Logging in…' : 'Log in'}
        </button>
      </form>
      <p className="mt-4 text-center text-sm text-zinc-500">
        New here?{' '}
        <Link to="/signup" className="font-medium text-sky-600 hover:underline">
          Create an account
        </Link>
      </p>
    </AuthLayout>
  )
}
