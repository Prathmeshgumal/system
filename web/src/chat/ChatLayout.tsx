import { useEffect, useState } from 'react'
import { Link, Outlet, useMatch } from 'react-router'
import { useAuth, useMe } from '../auth/AuthProvider'
import { notificationPermission, requestNotifications } from '../lib/notify'
import { getTheme, nextTheme, setTheme, type Theme } from '../lib/theme'
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

  // "(3) Relay" in the tab title, so unread messages show even in another tab.
  const totalUnread = chats.data?.reduce((n, c) => n + c.unread, 0) ?? 0
  useEffect(() => {
    document.title = totalUnread > 0 ? `(${totalUnread}) Relay` : 'Relay'
  }, [totalUnread])

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
          <ThemeButton />
          <button onClick={() => void logout()} className="text-sm text-zinc-500 hover:text-zinc-900 dark:hover:text-white">
            Log out
          </button>
        </header>
        <NotificationsPrompt />
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

const themeLabel: Record<Theme, string> = { system: 'Theme: Auto', light: 'Theme: Light', dark: 'Theme: Dark' }

function ThemeButton() {
  const [theme, setThemeState] = useState(getTheme)
  return (
    <button
      onClick={() => {
        const t = nextTheme[theme]
        setTheme(t)
        setThemeState(t)
      }}
      title="Theme: auto (follows your system), light or dark"
      className="text-xs whitespace-nowrap text-zinc-500 hover:text-zinc-900 dark:hover:text-white"
    >
      {themeLabel[theme]}
    </button>
  )
}

/** Asks for notification permission — only from a click; browsers block it otherwise. */
function NotificationsPrompt() {
  const [perm, setPerm] = useState(notificationPermission)
  if (perm !== 'default') return null
  return (
    <button
      onClick={async () => setPerm(await requestNotifications())}
      className="mx-3 mt-3 rounded-lg bg-sky-50 px-3 py-2 text-left text-sm text-sky-800 hover:bg-sky-100 dark:bg-sky-950 dark:text-sky-200"
    >
      🔔 Turn on notifications for new messages while Relay is in the background
    </button>
  )
}

export function NoChatSelected() {
  return (
    <div className="flex flex-1 items-center justify-center text-zinc-500">Pick a chat, or start a new one.</div>
  )
}
