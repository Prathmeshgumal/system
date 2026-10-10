import { useEffect, useRef, useState, useSyncExternalStore } from 'react'
import { Link, useParams } from 'react-router'
import { useMe } from '../auth/AuthProvider'
import { ApiError } from '../lib/api'
import { chatTitle } from '../lib/format'
import { Avatar } from '../ui/Avatar'
import { allMessages, maxSeq } from './cache'
import { Composer } from './Composer'
import { GroupInfo } from './GroupInfo'
import { MessageList } from './MessageList'
import { usePending } from './pending'
import { useChat, useChats, useDeleteMessage, useEditMessage, useMarkRead, useMessages, useSend } from './queries'

export function ChatView() {
  const id = Number(useParams().chatId)
  // key: switching chats remounts, so per-chat state (info panel, drafts) resets.
  return <ChatViewInner key={id} id={id} />
}

function ChatViewInner({ id }: { id: number }) {
  const me = useMe()
  const chat = useChat(id)
  const messages = useMessages(id)
  const pending = usePending(id)
  const send = useSend(id, me.id)
  const edit = useEditMessage(me.id)
  const del = useDeleteMessage(me.id)
  const markRead = useMarkRead()
  const listEntry = useChats().data?.find((c) => c.id === id)
  const visible = useDocumentVisible()
  const [showInfo, setShowInfo] = useState(false)

  // Seeing the newest message = having read it. Only while the tab is visible.
  const latest = maxSeq(messages.data)
  const lastRead = listEntry?.last_read_seq ?? 0
  const { mutate } = markRead
  const requested = useRef(0) // re-renders before the list cache updates mustn't re-send the same seq
  useEffect(() => {
    if (visible && latest > lastRead && latest > requested.current) {
      requested.current = latest
      mutate({ chatId: id, seq: latest })
    }
  }, [visible, latest, lastRead, id, mutate])

  const err = chat.error ?? messages.error
  if (err instanceof ApiError && err.status === 404) {
    return (
      <div className="flex flex-1 flex-col items-center justify-center gap-2 text-zinc-500">
        <p>This chat isn't available.</p>
        <Link to="/" className="text-sky-600 hover:underline">
          Back to chats
        </Link>
      </div>
    )
  }
  if (!chat.data) {
    return <div className="flex flex-1 items-center justify-center text-zinc-500">{err ? "Couldn't load this chat." : 'Loading…'}</div>
  }

  const c = chat.data
  const title = chatTitle(c)
  return (
    <div className="flex min-h-0 flex-1">
      <section className="flex min-w-0 flex-1 flex-col" aria-label={title}>
        <header className="flex items-center gap-3 border-b border-zinc-200 px-4 py-3 dark:border-zinc-800">
          <Link to="/" className="text-zinc-500 md:hidden" aria-label="Back">
            ←
          </Link>
          <Avatar name={title} id={c.kind === 'direct' ? (c.other_user?.id ?? c.id) : c.id} size="sm" />
          <div className="min-w-0 flex-1">
            <h1 className="truncate font-semibold">{title}</h1>
            <p className="truncate text-xs text-zinc-500">
              {c.kind === 'group' ? `${c.member_count} members` : c.other_user ? `@${c.other_user.username}` : ''}
            </p>
          </div>
          {c.kind === 'group' && (
            <button onClick={() => setShowInfo(!showInfo)} className="text-sm text-sky-600 hover:underline">
              {showInfo ? 'Close' : 'Group info'}
            </button>
          )}
        </header>
        {messages.isPending ? (
          <div className="flex flex-1 items-center justify-center text-zinc-500">Loading messages…</div>
        ) : (
          <MessageList
            messages={allMessages(messages.data)}
            pending={pending}
            me={me.id}
            isGroup={c.kind === 'group'}
            hasOlder={messages.hasNextPage}
            loadingOlder={messages.isFetchingNextPage}
            loadOlder={() => void messages.fetchNextPage()}
            onEdit={(mid, body) => edit.mutate({ id: mid, body })}
            onDelete={(mid) => del.mutate(mid)}
            onRetry={(p) => void send(p.body, p.client_id)}
          />
        )}
        <Composer onSend={(body) => void send(body)} />
      </section>
      {showInfo && c.kind === 'group' && <GroupInfo chat={c} onClose={() => setShowInfo(false)} />}
    </div>
  )
}

function subscribeVisibility(cb: () => void) {
  document.addEventListener('visibilitychange', cb)
  return () => document.removeEventListener('visibilitychange', cb)
}

function useDocumentVisible() {
  return useSyncExternalStore(subscribeVisibility, () => document.visibilityState === 'visible')
}
