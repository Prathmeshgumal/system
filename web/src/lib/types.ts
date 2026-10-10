// Shapes of the API's JSON. Mirrors the Go structs in backend/internal/{users,chat}.

export type Profile = {
  id: number
  username: string
  display_name: string
  bio: string
  created_at: string
}

/** My own account: a Profile plus my email. */
export type User = Profile & { email: string }

export type AuthResponse = {
  access_token: string
  expires_at: string
  user: User
}

export type Reaction = { emoji: string; user_ids: number[] }

/** The short version of a replied-to message, as it is now (edits/deletes show). */
export type Quote = {
  id: number
  seq: number
  sender: Profile | null
  body: string
  deleted: boolean
}

export type Message = {
  id: number
  chat_id: number
  seq: number
  sender_id: number
  body: string
  client_id: string
  created_at: string
  edited_at: string | null
  deleted_at: string | null
  sender: Profile | null
  reply_to_id?: number | null
  reply_to?: Quote | null
  reactions?: Reaction[]
}

/** The fixed reaction set — mirrors chat.Emojis in backend/internal/chat/service.go. */
export const EMOJIS = ['👍', '❤️', '😂', '😮', '😢', '🙏'] as const

export type Member = {
  user_id: number
  role: 'admin' | 'member'
  joined_at: string
  last_read_seq: number
  user: Profile
}

export type Chat = {
  id: number
  kind: 'direct' | 'group'
  name: string
  created_by: number
  created_at: string
  last_seq: number
  last_message_at: string | null
  my_role: 'admin' | 'member'
  last_read_seq: number
  unread: number
  member_count: number
  other_user?: Profile
  last_message: Message | null
  members?: Member[]
}

export type MessagePage = {
  messages: Message[] // oldest → newest
  has_more: boolean
}

/** Pushed over the WebSocket. See knowledge-base/adr/0003-websocket-protocol.md. */
export type ServerEvent =
  | { type: 'message.created' | 'message.updated' | 'message.deleted'; chat_id: number; message: Message }
  | { type: 'chat.created' | 'chat.updated' | 'chat.removed'; chat_id: number }
  | { type: 'chat.read'; chat_id: number; seq: number; user_id: number }
  | { type: 'typing'; chat_id: number; user_id: number }
