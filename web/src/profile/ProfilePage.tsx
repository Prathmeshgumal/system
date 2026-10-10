import { useState, type FormEvent } from 'react'
import { Link } from 'react-router'
import { useAuth, useMe } from '../auth/AuthProvider'
import { api, ApiError } from '../lib/api'
import type { User } from '../lib/types'
import { Field } from '../ui/Field'
import { primaryButton } from '../ui/buttons'

export function ProfilePage() {
  const me = useMe()
  const { setUser } = useAuth()
  const [fields, setFields] = useState<Record<string, string>>({})
  const [saved, setSaved] = useState(false)
  const [busy, setBusy] = useState(false)

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    const form = new FormData(e.currentTarget)
    setBusy(true)
    setFields({})
    setSaved(false)
    try {
      const u = await api<User>('/api/me', {
        method: 'PATCH',
        body: {
          display_name: String(form.get('display_name')),
          username: String(form.get('username')),
          bio: String(form.get('bio')),
        },
      })
      setUser(u)
      setSaved(true)
    } catch (err) {
      setFields(err instanceof ApiError ? { ...err.fields, _: err.message } : { _: 'Something went wrong' })
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex-1 overflow-y-auto p-6">
      <Link to="/" className="text-sm text-sky-600 md:hidden">
        ← Chats
      </Link>
      <h1 className="mb-1 text-2xl font-bold">Your profile</h1>
      <p className="mb-6 text-sm text-zinc-500">{me.email}</p>
      <form onSubmit={submit} className="max-w-md space-y-4">
        <Field label="Display name" name="display_name" defaultValue={me.display_name} error={fields.display_name} />
        <Field label="Username" name="username" defaultValue={me.username} error={fields.username} />
        <div className="space-y-1">
          <label htmlFor="bio" className="block text-sm font-medium text-zinc-700 dark:text-zinc-300">
            Bio
          </label>
          <textarea
            id="bio"
            name="bio"
            defaultValue={me.bio}
            maxLength={280}
            rows={3}
            className="w-full rounded-lg border border-zinc-300 bg-white px-3 py-2 dark:border-zinc-700 dark:bg-zinc-900"
          />
          {fields.bio && <p className="text-sm text-rose-600">{fields.bio}</p>}
        </div>
        {fields._ && !fields.display_name && !fields.username && !fields.bio && (
          <p className="text-sm text-rose-600">{fields._}</p>
        )}
        {saved && <p className="text-sm text-emerald-600">Saved.</p>}
        <button type="submit" disabled={busy} className={primaryButton}>
          Save
        </button>
      </form>
    </div>
  )
}
