import { useState } from 'react'
import { Link, Outlet, useMatch } from 'react-router'
import { useAuth, useMe } from '../auth/AuthProvider'
import { useSocketStatus } from '../realtime/RealtimeProvider'
import { Avatar } from '../ui/Avatar'
import { secondaryButton } from '../ui/buttons'
import { ChatList } from './ChatList'
import { NewChatDialog } from './NewChatDialog'
import { useChats } from './queries'

export function ChatLayout() {
  const me = useMe()
  const { logout } = useAuth()
  const chats = useChats()
  const [dialog, setDialog] = useState<'direct' | 'group' | null>(null)
  // On small screens show either the list or the open chat, not both.
  const chatOpen = useMatch('/chats/:chatId')
  const profileOpen = useMatch('/profile')
  const inChat = chatOpen !== null || profileOpen !== null

  return (
    <div className="flex h-dvh bg-white text-zinc-900 dark:bg-zinc-950 dark:text-zinc-100">
      <aside
        className={`${inChat ? 'hidden md:flex' : 'flex'} w-full flex-col border-r border-zinc-200 md:w-80 dark:border-zinc-800`}
      >
        <header className="flex items-center gap-3 border-b border-zinc-200 p-3 dark:border-zinc-800">
          <Link to="/profile" className="flex min-w-0 flex-1 items-center gap-3" title="Your profile">
            <Avatar name={me.display_name} id={me.id} size="sm" />
            <span className="min-w-0">
              <span className="block truncate font-semibold">{me.display_name}</span>
              <ConnectionStatus />
            </span>
          </Link>
          <button onClick={() => void logout()} className="text-sm text-zinc-500 hover:text-zinc-900 dark:hover:text-white">
            Log out
          </button>
        </header>
        <div className="flex gap-2 p-3">
          <button onClick={() => setDialog('direct')} className={`${secondaryButton} flex-1`}>
            New chat
          </button>
          <button onClick={() => setDialog('group')} className={`${secondaryButton} flex-1`}>
            New group
          </button>
        </div>
        <nav aria-label="Chats" className="flex-1 overflow-y-auto">
          {chats.isPending ? (
            <p className="p-4 text-sm text-zinc-500">Loading chats…</p>
          ) : chats.isError ? (
            <p className="p-4 text-sm text-rose-600">Couldn't load chats.</p>
          ) : (
            <ChatList chats={chats.data} me={me.id} />
          )}
        </nav>
      </aside>
      <main className={`${inChat ? 'flex' : 'hidden md:flex'} min-w-0 flex-1 flex-col`}>
        <Outlet />
      </main>
      {dialog && <NewChatDialog mode={dialog} onClose={() => setDialog(null)} />}
    </div>
  )
}

function ConnectionStatus() {
  const status = useSocketStatus()
  const label = { open: 'Connected', connecting: 'Connecting…', reconnecting: 'Reconnecting…' }[status]
  return (
    <span className="flex items-center gap-1.5 text-xs text-zinc-500" role="status">
      <span className={`h-2 w-2 rounded-full ${status === 'open' ? 'bg-emerald-500' : 'bg-amber-500'}`} />
      {label}
    </span>
  )
}

export function NoChatSelected() {
  return (
    <div className="flex flex-1 items-center justify-center text-zinc-500">Pick a chat, or start a new one.</div>
  )
}
