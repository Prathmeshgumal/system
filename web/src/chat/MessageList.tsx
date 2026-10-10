import { useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { clockTime } from '../lib/format'
import type { Message } from '../lib/types'
import type { Pending } from './pending'

type Props = {
  messages: Message[]
  pending: Pending[]
  me: number
  isGroup: boolean
  hasOlder: boolean
  loadingOlder: boolean
  loadOlder: () => void
  onEdit: (id: number, body: string) => void
  onDelete: (id: number) => void
  onRetry: (p: Pending) => void
}

/**
 * The scroll container is flex-col-reverse: the browser treats the BOTTOM as
 * the start, so the view stays pinned to the newest message as messages arrive,
 * and loading older pages at the top doesn't make it jump.
 */
export function MessageList(props: Props) {
  const { messages, pending, me, isGroup, hasOlder, loadingOlder, loadOlder } = props
  const top = useRef<HTMLLIElement>(null)

  // Infinite scroll: when the top of the list comes into view, load the page before it.
  useEffect(() => {
    const el = top.current
    if (!el || !hasOlder || typeof IntersectionObserver === 'undefined') return
    const io = new IntersectionObserver((entries) => {
      if (entries[0].isIntersecting && !loadingOlder) loadOlder()
    })
    io.observe(el)
    return () => io.disconnect()
  }, [hasOlder, loadingOlder, loadOlder])

  return (
    <div className="flex flex-1 flex-col-reverse overflow-y-auto px-4 py-3">
      <ol className="space-y-1" aria-label="Messages">
        {hasOlder && (
          <li ref={top} className="py-2 text-center">
            <button onClick={loadOlder} disabled={loadingOlder} className="text-sm text-sky-600 hover:underline">
              {loadingOlder ? 'Loading…' : 'Load older messages'}
            </button>
          </li>
        )}
        {messages.map((m, i) => (
          <MessageItem
            key={m.seq}
            m={m}
            mine={m.sender_id === me}
            showSender={isGroup && m.sender_id !== me && messages[i - 1]?.sender_id !== m.sender_id}
            onEdit={props.onEdit}
            onDelete={props.onDelete}
          />
        ))}
        {pending.map((p) => (
          <li key={p.client_id} className="flex justify-end">
            <div className="max-w-[75%] rounded-2xl bg-sky-600/60 px-3 py-2 whitespace-pre-wrap text-white">
              {p.body}
              <div className="mt-0.5 text-right text-xs text-white/80">
                {p.status === 'sending' ? (
                  'Sending…'
                ) : (
                  <button onClick={() => props.onRetry(p)} className="underline">
                    Failed — retry
                  </button>
                )}
              </div>
            </div>
          </li>
        ))}
      </ol>
    </div>
  )
}

function MessageItem({
  m,
  mine,
  showSender,
  onEdit,
  onDelete,
}: {
  m: Message
  mine: boolean
  showSender: boolean
  onEdit: (id: number, body: string) => void
  onDelete: (id: number) => void
}) {
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(m.body)
  const deleted = m.deleted_at !== null

  function onKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Escape') setEditing(false)
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      if (draft.trim() && draft !== m.body) onEdit(m.id, draft)
      setEditing(false)
    }
  }

  return (
    <li className={`group flex flex-col ${mine ? 'items-end' : 'items-start'}`}>
      {showSender && <span className="mt-2 ml-3 text-xs font-medium text-zinc-500">{m.sender?.display_name ?? 'Deleted account'}</span>}
      <div className="flex max-w-[75%] items-center gap-2">
        {mine && !deleted && !editing && (
          <span className="hidden gap-1 text-xs group-hover:flex">
            <button
              onClick={() => {
                setDraft(m.body)
                setEditing(true)
              }}
              className="text-zinc-500 hover:text-zinc-900 dark:hover:text-white"
            >
              Edit
            </button>
            <button onClick={() => onDelete(m.id)} className="text-rose-500 hover:text-rose-700">
              Delete
            </button>
          </span>
        )}
        <div
          className={`rounded-2xl px-3 py-2 whitespace-pre-wrap break-words ${
            mine ? 'bg-sky-600 text-white' : 'bg-zinc-100 dark:bg-zinc-800'
          } ${deleted ? 'italic opacity-60' : ''}`}
        >
          {editing ? (
            <textarea
              autoFocus
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={onKeyDown}
              onBlur={() => setEditing(false)}
              aria-label="Edit message"
              className="w-64 resize-none bg-transparent outline-none"
            />
          ) : deleted ? (
            'Message deleted'
          ) : (
            m.body
          )}
          <div className={`mt-0.5 text-right text-xs ${mine ? 'text-white/70' : 'text-zinc-500'}`}>
            {m.edited_at && !deleted && 'edited · '}
            {clockTime(m.created_at)}
          </div>
        </div>
      </div>
    </li>
  )
}
