// What to do with each WebSocket event, and how to catch up after a reconnect.
import type { QueryClient } from '@tanstack/react-query'
import { api, ApiError } from '../lib/api'
import type { Chat, MessagePage, ServerEvent } from '../lib/types'
import { applyReadToList, maxSeq, upsertMessage, type MessagesData } from '../chat/cache'
import { keys, storeMessage } from '../chat/queries'

export function handleEvent(qc: QueryClient, e: ServerEvent, me: number) {
  switch (e.type) {
    case 'message.created':
      return storeMessage(qc, e.message, me, 'created')
    case 'message.updated':
      return storeMessage(qc, e.message, me, 'updated')
    case 'message.deleted':
      return storeMessage(qc, e.message, me, 'deleted')
    case 'chat.created':
    case 'chat.updated':
      void qc.invalidateQueries({ queryKey: keys.chats })
      void qc.invalidateQueries({ queryKey: keys.chat(e.chat_id) })
      return
    case 'chat.removed':
      void qc.invalidateQueries({ queryKey: keys.chats })
      void qc.invalidateQueries({ queryKey: keys.chat(e.chat_id) }) // the open view gets a 404 → "not available"
      qc.removeQueries({ queryKey: keys.messages(e.chat_id) })
      return
    case 'chat.read':
      qc.setQueryData<Chat[]>(keys.chats, (l) => applyReadToList(l, e.chat_id, e.seq))
      return
  }
}

/**
 * Runs on every (re)connect — including the first: anything sent between the
 * page's first fetch and the socket opening would otherwise be missed.
 *
 * New messages: for each chat with loaded history, ask for everything after
 * the newest seq we have. seq is commit-ordered per chat (ADR 0002), so this
 * can't skip anything.
 *
 * Known gap: edits/deletes of OLDER messages during the outage aren't in an
 * "after" page. We mark history stale so a chat refetches fully when reopened.
 */
export async function catchUp(qc: QueryClient) {
  void qc.invalidateQueries({ queryKey: keys.chats })
  void qc.invalidateQueries({ queryKey: ['chat'] })

  const queries = qc.getQueryCache().findAll({ queryKey: ['messages'] })
  await Promise.all(
    queries.map(async (q) => {
      const chatId = q.queryKey[1] as number
      if (!q.state.data) return
      let after = maxSeq(q.state.data as MessagesData)
      for (let i = 0; i < 20; i++) {
        let page: MessagePage
        try {
          page = await api<MessagePage>(`/api/chats/${chatId}/messages?after=${after}&limit=100`)
        } catch (err) {
          if (err instanceof ApiError && err.status === 404) qc.removeQueries({ queryKey: keys.messages(chatId) })
          return
        }
        qc.setQueryData<MessagesData>(keys.messages(chatId), (d) => page.messages.reduce(upsertMessage, d))
        if (!page.has_more || page.messages.length === 0) return
        after = page.messages[page.messages.length - 1].seq
      }
      // Way behind (2000+ messages): simpler to start this chat over.
      void qc.resetQueries({ queryKey: keys.messages(chatId) })
    }),
  )
  void qc.invalidateQueries({ queryKey: ['messages'], refetchType: 'none' })
}
