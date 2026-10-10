// Read receipts are derived, not stored per message: a member has seen
// message N if their last_read_seq ≥ N (ADR 0002 already keeps last_read_seq).
import type { Chat } from '../lib/types'

/** 1:1 chats: ✓ sent, ✓✓ seen by the other person. */
export function directReceipt(chat: Chat, seq: number, me: number): 'sent' | 'seen' {
  const other = chat.members?.find((m) => m.user_id !== me)
  return other && other.last_read_seq >= seq ? 'seen' : 'sent'
}

/** Groups: how many other members have read up to seq. */
export function seenBy(chat: Chat, seq: number, me: number): number {
  return chat.members?.filter((m) => m.user_id !== me && m.last_read_seq >= seq).length ?? 0
}

/** Applies a chat.read event to a chat's member list (never moves backwards). */
export function applyMemberRead(chat: Chat | undefined, userId: number, seq: number): Chat | undefined {
  if (!chat?.members) return chat
  return {
    ...chat,
    members: chat.members.map((m) =>
      m.user_id === userId ? { ...m, last_read_seq: Math.max(m.last_read_seq, seq) } : m,
    ),
  }
}
