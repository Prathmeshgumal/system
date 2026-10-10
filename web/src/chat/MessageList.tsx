import { useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { clockTime } from '../lib/format'
import { EMOJIS, type Chat, type Message } from '../lib/types'
import type { Pending } from './pending'
import { directReceipt, seenBy } from './receipts'

type Props = {
  chat: Chat
  messages: Message[]
  pending: Pending[]
  me: number
  hasOlder: boolean
  loadingOlder: boolean
  loadOlder: () => void
  onEdit: (id: number, body: string) => void
  onDelete: (id: number) => void
  onRetry: (p: Pending) => void
  onReply: (m: Message) => void
  onReact: (m: Message, emoji: string, on: boolean) => void
}

/**
 * The scroll container is flex-col-reverse: the browser treats the BOTTOM as
 * the start, so the view stays pinned to the newest message as messages arrive,
 * and loading older pages at the top doesn't make it jump.
 */
export function MessageList(props: Props) {
  const { chat, messages, pending, me, hasOlder, loadingOlder, loadOlder } = props
  const top = useRef<HTMLLIElement>(null)
  const isGroup = chat.kind === 'group'

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

  // "Seen by N" goes under my newest message only (groups).
  const myLatest = messages.findLast((m) => m.sender_id === me && !m.deleted_at)

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
        {messages.map((m, i) => {
          const mine = m.sender_id === me
          let receipt: string | undefined
          if (mine && !m.deleted_at) {
            if (!isGroup) receipt = directReceipt(chat, m.seq, me) === 'seen' ? '✓✓' : '✓'
            else if (m === myLatest) {
              const n = seenBy(chat, m.seq, me)
              receipt = n > 0 ? `Seen by ${n}` : undefined
            }
          }
          return (
            <MessageItem
              key={m.seq}
              m={m}
              me={me}
              mine={mine}
              showSender={isGroup && !mine && messages[i - 1]?.sender_id !== m.sender_id}
              receipt={receipt}
              onEdit={props.onEdit}
              onDelete={props.onDelete}
              onReply={props.onReply}
              onReact={props.onReact}
            />
          )
        })}
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

type ItemProps = {
  m: Message
  me: number
  mine: boolean
  showSender: boolean
  receipt?: string
  onEdit: (id: number, body: string) => void
  onDelete: (id: number) => void
  onReply: (m: Message) => void
  onReact: (m: Message, emoji: string, on: boolean) => void
}

function MessageItem({ m, me, mine, showSender, receipt, onEdit, onDelete, onReply, onReact }: ItemProps) {
  const [editing, setEditing] = useState(false)
  const [picking, setPicking] = useState(false)
  const [draft, setDraft] = useState(m.body)
  const deleted = m.deleted_at !== null
  const reactions = m.reactions ?? []

  function onKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Escape') setEditing(false)
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      if (draft.trim() && draft !== m.body) onEdit(m.id, draft)
      setEditing(false)
    }
  }

  const actions = !deleted && !editing && (
    <span className="hidden gap-2 text-xs group-hover:flex group-focus-within:flex">
      <button onClick={() => onReply(m)} className="text-zinc-500 hover:text-zinc-900 dark:hover:text-white">
        Reply
      </button>
      <button
        onClick={() => setPicking(!picking)}
        aria-label="React"
        className="text-zinc-500 hover:text-zinc-900 dark:hover:text-white"
      >
        React
      </button>
      {mine && (
        <>
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
        </>
      )}
    </span>
  )

  return (
    <li id={`msg-${m.seq}`} className={`group flex flex-col ${mine ? 'items-end' : 'items-start'}`}>
      {showSender && (
        <span className="mt-2 ml-3 text-xs font-medium text-zinc-500">{m.sender?.display_name ?? 'Deleted account'}</span>
      )}
      <div className={`flex max-w-[75%] items-center gap-2 ${mine ? '' : 'flex-row-reverse'}`}>
        {actions}
        <div
          className={`min-w-0 rounded-2xl px-3 py-2 break-words whitespace-pre-wrap ${
            mine ? 'bg-sky-600 text-white' : 'bg-zinc-100 dark:bg-zinc-800'
          } ${deleted ? 'italic opacity-60' : ''}`}
        >
          {m.reply_to && !deleted && (
            <button
              onClick={() => document.getElementById(`msg-${m.reply_to!.seq}`)?.scrollIntoView({ block: 'center' })}
              className={`mb-1 block w-full rounded-lg border-l-4 px-2 py-1 text-left text-xs ${
                mine ? 'border-white/60 bg-white/15' : 'border-sky-500 bg-white/60 dark:bg-zinc-900/60'
              }`}
              aria-label="Replied-to message"
            >
              <span className="block font-semibold">{m.reply_to.sender?.display_name ?? 'Deleted account'}</span>
              <span className="line-clamp-2 opacity-80">{m.reply_to.deleted ? 'Message deleted' : m.reply_to.body}</span>
            </button>
          )}
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
            {receipt && receipt.startsWith('✓') && (
              <span aria-label={receipt === '✓✓' ? 'Seen' : 'Sent'} className="ml-1">
                {receipt}
              </span>
            )}
          </div>
        </div>
      </div>
      {picking && (
        <div role="group" aria-label="Pick a reaction" className="mt-1 flex gap-1 rounded-full bg-white px-2 py-1 shadow ring-1 ring-zinc-200 dark:bg-zinc-800 dark:ring-zinc-700">
          {EMOJIS.map((e) => {
            const on = reactions.some((r) => r.emoji === e && r.user_ids.includes(me))
            return (
              <button
                key={e}
                onClick={() => {
                  onReact(m, e, !on)
                  setPicking(false)
                }}
                className={`rounded-full px-1 text-lg hover:scale-125 ${on ? 'bg-sky-100 dark:bg-sky-900' : ''}`}
              >
                {e}
              </button>
            )
          })}
        </div>
      )}
      {reactions.length > 0 && (
        <div className="mt-0.5 flex flex-wrap gap-1">
          {reactions.map((r) => {
            const on = r.user_ids.includes(me)
            return (
              <button
                key={r.emoji}
                onClick={() => onReact(m, r.emoji, !on)}
                aria-pressed={on}
                aria-label={`${r.emoji} ${r.user_ids.length}`}
                className={`rounded-full border px-2 py-0.5 text-xs ${
                  on
                    ? 'border-sky-400 bg-sky-50 dark:border-sky-600 dark:bg-sky-950'
                    : 'border-zinc-200 bg-white dark:border-zinc-700 dark:bg-zinc-900'
                }`}
              >
                {r.emoji} {r.user_ids.length}
              </button>
            )
          })}
        </div>
      )}
      {receipt && !receipt.startsWith('✓') && <span className="mt-0.5 text-xs text-zinc-500">{receipt}</span>}
    </li>
  )
}
