import { NavLink } from 'react-router'
import { chatTitle, shortTime } from '../lib/format'
import type { Chat } from '../lib/types'
import { Avatar } from '../ui/Avatar'

function preview(c: Chat, me: number): string {
  const m = c.last_message
  if (!m) return c.kind === 'group' ? `${c.member_count} members` : 'Say hi 👋'
  if (m.deleted_at) return 'Message deleted'
  const who = m.sender_id === me ? 'You: ' : c.kind === 'group' && m.sender ? `${m.sender.display_name}: ` : ''
  return who + m.body
}

export function ChatList({ chats, me }: { chats: Chat[]; me: number }) {
  if (chats.length === 0) {
    return <p className="p-4 text-sm text-zinc-500">No chats yet. Start one with the buttons above.</p>
  }
  return (
    <ul>
      {chats.map((c) => (
        <li key={c.id}>
          <NavLink
            to={`/chats/${c.id}`}
            className={({ isActive }) =>
              `flex items-center gap-3 px-3 py-2.5 hover:bg-zinc-100 dark:hover:bg-zinc-800/60 ${isActive ? 'bg-zinc-100 dark:bg-zinc-800' : ''}`
            }
          >
            <Avatar name={chatTitle(c)} id={c.kind === 'direct' ? (c.other_user?.id ?? c.id) : c.id} />
            <div className="min-w-0 flex-1">
              <div className="flex items-baseline justify-between gap-2">
                <span className="truncate font-medium">{chatTitle(c)}</span>
                <span className="shrink-0 text-xs text-zinc-500">{shortTime(c.last_message_at ?? c.created_at)}</span>
              </div>
              <div className="flex items-center justify-between gap-2">
                <span className="truncate text-sm text-zinc-500">{preview(c, me)}</span>
                {c.unread > 0 && (
                  <span
                    aria-label={`${c.unread} unread`}
                    className="shrink-0 rounded-full bg-sky-600 px-2 py-0.5 text-xs font-semibold text-white"
                  >
                    {c.unread > 99 ? '99+' : c.unread}
                  </span>
                )}
              </div>
            </div>
          </NavLink>
        </li>
      ))}
    </ul>
  )
}
