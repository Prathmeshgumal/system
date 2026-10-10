import { useState } from 'react'
import { useNavigate } from 'react-router'
import { useMe } from '../auth/AuthProvider'
import { api, ApiError } from '../lib/api'
import type { Chat } from '../lib/types'
import { Avatar } from '../ui/Avatar'
import { dangerButton, secondaryButton } from '../ui/buttons'
import { useChatMutation } from './queries'
import { UserSearch } from './UserSearch'

export function GroupInfo({ chat, onClose }: { chat: Chat; onClose: () => void }) {
  const me = useMe()
  const navigate = useNavigate()
  const isAdmin = chat.my_role === 'admin'
  const [name, setName] = useState(chat.name)
  const [adding, setAdding] = useState(false)
  const [error, setError] = useState('')
  const base = `/api/chats/${chat.id}`

  const onError = (err: unknown) => setError(err instanceof ApiError ? err.message : 'Something went wrong')
  const rename = useChatMutation(chat.id, (n: string) => api<Chat>(base, { method: 'PATCH', body: { name: n } }))
  const add = useChatMutation(chat.id, (id: number) =>
    api<Chat>(`${base}/members`, { method: 'POST', body: { user_ids: [id] } }),
  )
  const setRole = useChatMutation(chat.id, (v: { id: number; role: 'admin' | 'member' }) =>
    api<Chat>(`${base}/members/${v.id}`, { method: 'PATCH', body: { role: v.role } }),
  )
  const remove = useChatMutation(chat.id, (id: number) => api<void>(`${base}/members/${id}`, { method: 'DELETE' }))

  return (
    <aside className="flex w-72 shrink-0 flex-col gap-4 overflow-y-auto border-l border-zinc-200 p-4 dark:border-zinc-800">
      <div className="flex items-center justify-between">
        <h2 className="font-semibold">Group info</h2>
        <button onClick={onClose} aria-label="Close group info" className="text-zinc-500">
          ✕
        </button>
      </div>

      {isAdmin ? (
        <form
          onSubmit={(e) => {
            e.preventDefault()
            rename.mutate(name, { onError })
          }}
          className="flex gap-2"
        >
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            aria-label="Group name"
            maxLength={64}
            className="min-w-0 flex-1 rounded-lg border border-zinc-300 bg-white px-2 py-1 dark:border-zinc-700 dark:bg-zinc-900"
          />
          <button type="submit" disabled={name.trim() === chat.name || !name.trim()} className={secondaryButton}>
            Rename
          </button>
        </form>
      ) : (
        <p className="font-medium">{chat.name}</p>
      )}

      <div>
        <div className="mb-2 flex items-center justify-between">
          <h3 className="text-sm font-semibold text-zinc-500">{chat.member_count} members</h3>
          {isAdmin && (
            <button onClick={() => setAdding(!adding)} className="text-sm text-sky-600 hover:underline">
              {adding ? 'Done' : 'Add people'}
            </button>
          )}
        </div>
        {adding && (
          <div className="mb-3">
            <UserSearch
              onPick={(p) => add.mutate(p.id, { onError })}
              exclude={chat.members?.map((m) => m.user_id) ?? []}
            />
          </div>
        )}
        <ul className="space-y-2">
          {chat.members?.map((m) => (
            <li key={m.user_id} className="flex items-center gap-2">
              <Avatar name={m.user.display_name} id={m.user_id} url={m.user.avatar_url} size="sm" />
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium">
                  {m.user.display_name}
                  {m.user_id === me.id && ' (you)'}
                </p>
                <p className="text-xs text-zinc-500">{m.role === 'admin' ? 'Admin' : `@${m.user.username}`}</p>
              </div>
              {isAdmin && m.user_id !== me.id && (
                <div className="flex flex-col items-end text-xs">
                  <button
                    onClick={() =>
                      setRole.mutate({ id: m.user_id, role: m.role === 'admin' ? 'member' : 'admin' }, { onError })
                    }
                    className="text-sky-600 hover:underline"
                  >
                    {m.role === 'admin' ? 'Remove admin' : 'Make admin'}
                  </button>
                  <button onClick={() => remove.mutate(m.user_id, { onError })} className="text-rose-600 hover:underline">
                    Remove
                  </button>
                </div>
              )}
            </li>
          ))}
        </ul>
      </div>

      {error && <p className="text-sm text-rose-600">{error}</p>}

      <button
        onClick={() =>
          remove.mutate(me.id, {
            onSuccess: () => navigate('/'),
            onError,
          })
        }
        className={`${dangerButton} mt-auto`}
      >
        Leave group
      </button>
    </aside>
  )
}
