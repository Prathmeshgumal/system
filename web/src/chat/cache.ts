// Pure functions that apply a change (a new message, an edit, a read marker)
// to what TanStack Query has cached — so a WebSocket event updates the screen
// without refetching anything. Pure = easy to test: see cache.test.ts.
import type { InfiniteData } from '@tanstack/react-query'
import type { Chat, Message, MessagePage } from '../lib/types'

/** The ['messages', chatId] cache: pages[0] is the newest page; each page is oldest → newest. */
export type MessagesData = InfiniteData<MessagePage, unknown>

export function allMessages(data: MessagesData | undefined): Message[] {
  if (!data) return []
  return data.pages.toReversed().flatMap((p) => p.messages)
}

export function maxSeq(data: MessagesData | undefined): number {
  const newest = data?.pages[0]?.messages
  return newest?.length ? newest[newest.length - 1].seq : 0
}

/**
 * Adds or replaces a message, matched by seq:
 * - already loaded → replaced in place (edits, deletes, duplicate deliveries)
 * - newer than the newest loaded → inserted into the newest page, in seq order
 * - older than everything loaded → ignored; scrolling up will fetch it
 */
export function upsertMessage(data: MessagesData | undefined, m: Message): MessagesData | undefined {
  if (!data) return data

  for (let i = 0; i < data.pages.length; i++) {
    const at = data.pages[i].messages.findIndex((x) => x.seq === m.seq)
    if (at >= 0) {
      const messages = data.pages[i].messages.with(at, m)
      return { ...data, pages: data.pages.with(i, { ...data.pages[i], messages }) }
    }
  }

  const oldestLoaded = allMessages(data)[0]
  if (oldestLoaded && m.seq < oldestLoaded.seq) return data

  const first = data.pages[0] ?? { messages: [], has_more: false }
  const messages = [...first.messages, m].sort((a, b) => a.seq - b.seq)
  const pages = data.pages.length ? data.pages.with(0, { ...first, messages }) : [{ ...first, messages }]
  return { ...data, pages }
}

/**
 * Applies a message event to the chat list. `found` is false when the chat
 * isn't in the list yet (e.g. just added to a group) — the caller refetches.
 */
export function applyMessageToList(
  chats: Chat[] | undefined,
  m: Message,
  me: number,
  kind: 'created' | 'updated' | 'deleted',
): { chats: Chat[] | undefined; found: boolean } {
  if (!chats) return { chats, found: false }
  const i = chats.findIndex((c) => c.id === m.chat_id)
  if (i < 0) return { chats, found: false }
  const c = chats[i]

  if (kind !== 'created') {
    if (c.last_message?.id !== m.id) return { chats, found: true }
    return { chats: chats.with(i, { ...c, last_message: m }), found: true }
  }

  const last_seq = Math.max(c.last_seq, m.seq)
  const last_read_seq = m.sender_id === me ? Math.max(c.last_read_seq, m.seq) : c.last_read_seq
  const newer = m.seq >= c.last_seq
  const updated: Chat = {
    ...c,
    last_seq,
    last_read_seq,
    unread: last_seq - last_read_seq,
    last_message: newer ? m : c.last_message,
    last_message_at: newer ? m.created_at : c.last_message_at,
  }
  // Newest activity first: move it to the top.
  return { chats: [updated, ...chats.toSpliced(i, 1)], found: true }
}

export function applyReadToList(chats: Chat[] | undefined, chatId: number, seq: number): Chat[] | undefined {
  if (!chats) return chats
  return chats.map((c) => {
    if (c.id !== chatId) return c
    const last_read_seq = Math.min(Math.max(c.last_read_seq, seq), c.last_seq)
    return { ...c, last_read_seq, unread: c.last_seq - last_read_seq }
  })
}
