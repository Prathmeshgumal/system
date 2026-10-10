import { useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useNavigate } from 'react-router'
import { api, ApiError } from '../lib/api'
import type { Chat, Profile } from '../lib/types'
import { Modal } from '../ui/Modal'
import { primaryButton } from '../ui/buttons'
import { keys } from './queries'
import { UserSearch } from './UserSearch'

export function NewChatDialog({ mode, onClose }: { mode: 'direct' | 'group'; onClose: () => void }) {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [name, setName] = useState('')
  const [picked, setPicked] = useState<Profile[]>([])
  const [error, setError] = useState('')

  async function open(promise: Promise<Chat>) {
    try {
      const chat = await promise
      qc.setQueryData(keys.chat(chat.id), chat)
      void qc.invalidateQueries({ queryKey: keys.chats })
      onClose()
      navigate(`/chats/${chat.id}`)
    } catch (err) {
      setError(err instanceof ApiError ? (Object.values(err.fields)[0] ?? err.message) : 'Something went wrong')
    }
  }

  if (mode === 'direct') {
    return (
      <Modal title="New chat" onClose={onClose}>
        <UserSearch
          onPick={(p) => open(api<Chat>('/api/chats/direct', { method: 'POST', body: { user_id: p.id } }))}
        />
        {error && <p className="mt-2 text-sm text-rose-600">{error}</p>}
      </Modal>
    )
  }

  return (
    <Modal title="New group" onClose={onClose}>
      <div className="space-y-3">
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Group name"
          aria-label="Group name"
          maxLength={64}
          className="w-full rounded-lg border border-zinc-300 bg-white px-3 py-2 outline-none focus:border-sky-500 dark:border-zinc-700 dark:bg-zinc-900"
        />
        {picked.length > 0 && (
          <div className="flex flex-wrap gap-2">
            {picked.map((p) => (
              <button
                key={p.id}
                onClick={() => setPicked(picked.filter((x) => x.id !== p.id))}
                className="rounded-full bg-sky-100 px-3 py-1 text-sm text-sky-800 dark:bg-sky-900 dark:text-sky-100"
                title="Remove"
              >
                {p.display_name} ✕
              </button>
            ))}
          </div>
        )}
        <UserSearch onPick={(p) => setPicked([...picked, p])} exclude={picked.map((p) => p.id)} />
        {error && <p className="text-sm text-rose-600">{error}</p>}
        <button
          disabled={!name.trim() || picked.length === 0}
          onClick={() =>
            open(
              api<Chat>('/api/chats/group', {
                method: 'POST',
                body: { name, member_ids: picked.map((p) => p.id) },
              }),
            )
          }
          className={`${primaryButton} w-full`}
        >
          Create group
        </button>
      </div>
    </Modal>
  )
}
