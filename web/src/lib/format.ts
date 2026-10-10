import type { Chat } from './types'

const time = new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit' })
const day = new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric' })

/** "14:05" today, "Mar 3" otherwise. */
export function shortTime(iso: string | null | undefined, now = new Date()): string {
  if (!iso) return ''
  const d = new Date(iso)
  return d.toDateString() === now.toDateString() ? time.format(d) : day.format(d)
}

export function clockTime(iso: string): string {
  return time.format(new Date(iso))
}

export function chatTitle(c: Pick<Chat, 'kind' | 'name' | 'other_user'>): string {
  return c.kind === 'group' ? c.name : (c.other_user?.display_name ?? 'Deleted account')
}

export function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  return (parts.length > 1 ? parts[0][0] + parts[1][0] : name.slice(0, 2)).toUpperCase()
}
