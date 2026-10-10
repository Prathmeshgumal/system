import { useState, type FormEvent } from 'react'
import { Link, Navigate } from 'react-router'
import { ApiError } from '../lib/api'
import { Field } from '../ui/Field'
import { primaryButton } from '../ui/buttons'
import { AuthLayout } from './AuthLayout'
import { useAuth } from './AuthProvider'

export function SignupPage() {
  const { state, signup } = useAuth()
  const [fields, setFields] = useState<Record<string, string>>({})
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  if (state.status === 'in') return <Navigate to="/" replace />

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    const form = new FormData(e.currentTarget)
    setBusy(true)
    setFields({})
    setError('')
    try {
      await signup({
        email: String(form.get('email')),
        username: String(form.get('username')),
        display_name: String(form.get('display_name')),
        password: String(form.get('password')),
      })
    } catch (err) {
      if (err instanceof ApiError && Object.keys(err.fields).length > 0) setFields(err.fields)
      else setError(err instanceof ApiError ? err.message : 'Something went wrong')
    } finally {
      setBusy(false)
    }
  }

  return (
    <AuthLayout title="Create your account">
      <form onSubmit={submit} className="space-y-4" noValidate>
        <Field label="Email" name="email" type="email" autoComplete="email" error={fields.email} />
        <Field label="Username" name="username" autoComplete="username" error={fields.username} />
        <Field label="Display name" name="display_name" autoComplete="name" error={fields.display_name} />
        <Field label="Password" name="password" type="password" autoComplete="new-password" error={fields.password} />
        {error && (
          <p role="alert" className="text-sm text-rose-600">
            {error}
          </p>
        )}
        <button type="submit" disabled={busy} className={`${primaryButton} w-full`}>
          {busy ? 'Creating…' : 'Sign up'}
        </button>
      </form>
      <p className="mt-4 text-center text-sm text-zinc-500">
        Have an account?{' '}
        <Link to="/login" className="font-medium text-sky-600 hover:underline">
          Log in
        </Link>
      </p>
    </AuthLayout>
  )
}
