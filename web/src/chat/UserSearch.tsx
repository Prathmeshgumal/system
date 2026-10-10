import { useEffect, useState } from 'react'
import type { Profile } from '../lib/types'
import { Avatar } from '../ui/Avatar'
import { useUserSearch } from './queries'

/** Type a username prefix, pick a person. Debounced so each keystroke isn't a request. */
export function UserSearch({ onPick, exclude = [] }: { onPick: (p: Profile) => void; exclude?: number[] }) {
  const [text, setText] = useState('')
  const [q, setQ] = useState('')
  useEffect(() => {
    const t = setTimeout(() => setQ(text), 250)
    return () => clearTimeout(t)
  }, [text])
  const { data, isFetching } = useUserSearch(q)
  const results = (data ?? []).filter((p) => !exclude.includes(p.id))

  return (
    <div>
      <input
        autoFocus
        value={text}
        onChange={(e) => setText(e.target.value)}
        placeholder="Search by username…"
        aria-label="Search users"
        className="w-full rounded-lg border border-zinc-300 bg-white px-3 py-2 outline-none focus:border-sky-500 dark:border-zinc-700 dark:bg-zinc-900"
      />
      <ul className="mt-2 max-h-64 overflow-y-auto">
        {results.map((p) => (
          <li key={p.id}>
            <button
              onClick={() => onPick(p)}
              className="flex w-full items-center gap-3 rounded-lg px-2 py-2 text-left hover:bg-zinc-100 dark:hover:bg-zinc-800"
            >
              <Avatar name={p.display_name} id={p.id} url={p.avatar_url} size="sm" />
              <span>
                <span className="block font-medium">{p.display_name}</span>
                <span className="block text-sm text-zinc-500">@{p.username}</span>
              </span>
            </button>
          </li>
        ))}
        {text.trim() && q === text && !isFetching && results.length === 0 && <li className="px-2 py-2 text-sm text-zinc-500">No one found</li>}
      </ul>
    </div>
  )
}
