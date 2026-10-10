// Every server read and write for chats, as TanStack Query hooks.
// Query keys: ['chats'] the list · ['chat', id] one chat with members · ['messages', id] history pages.
import { useInfiniteQuery, useMutation, useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query'
import { api, ApiError } from '../lib/api'
import type { Chat, Message, MessagePage, Profile } from '../lib/types'
import { uuid } from '../lib/uuid'
import { applyMessageToList, applyReadToList, upsertMessage, type MessagesData } from './cache'
import { pendingStore } from './pending'

export const keys = {
  chats: ['chats'] as const,
  chat: (id: number) => ['chat', id] as const,
  messages: (id: number) => ['messages', id] as const,
  users: (q: string) => ['users', q] as const,
}

const notFound = (err: unknown) => err instanceof ApiError && err.status === 404
const retryUnless404 = (count: number, err: unknown) => !notFound(err) && count < 3

export function useChats() {
  return useQuery({
    queryKey: keys.chats,
    queryFn: ({ signal }) => api<{ chats: Chat[] }>('/api/chats', { signal }).then((r) => r.chats),
  })
}

export function useChat(id: number) {
  return useQuery({
    queryKey: keys.chat(id),
    queryFn: ({ signal }) => api<Chat>(`/api/chats/${id}`, { signal }),
    retry: retryUnless404,
  })
}

/** History, newest page first. Scrolling up loads the page before the oldest seq we have. */
export function useMessages(id: number) {
  return useInfiniteQuery({
    queryKey: keys.messages(id),
    queryFn: ({ pageParam, signal }) =>
      api<MessagePage>(`/api/chats/${id}/messages?limit=50${pageParam ? `&before=${pageParam}` : ''}`, { signal }),
    initialPageParam: undefined as number | undefined,
    getNextPageParam: (last) => (last.has_more && last.messages.length ? last.messages[0].seq : undefined),
    // Kept fresh by WebSocket events and reconnect catch-up, not by refetching.
    staleTime: Infinity,
    retry: retryUnless404,
  })
}

export function useUserSearch(q: string) {
  const query = q.trim().toLowerCase()
  return useQuery({
    queryKey: keys.users(query),
    queryFn: ({ signal }) =>
      api<{ users: Profile[] }>(`/api/users?q=${encodeURIComponent(query)}`, { signal }).then((r) => r.users),
    enabled: query.length > 0,
    staleTime: 30_000,
  })
}

/** Puts a confirmed message into the caches (history + chat list). */
export function storeMessage(qc: QueryClient, m: Message, me: number, kind: 'created' | 'updated' | 'deleted') {
  qc.setQueryData<MessagesData>(keys.messages(m.chat_id), (d) => upsertMessage(d, m))
  let found = true
  qc.setQueryData<Chat[]>(keys.chats, (list) => {
    const r = applyMessageToList(list, m, me, kind)
    found = r.found
    return r.chats
  })
  if (!found) void qc.invalidateQueries({ queryKey: keys.chats })
  pendingStore.remove(m.client_id)
}

/** Sends with an optimistic placeholder. Retrying reuses the client_id, so it can't duplicate. */
export function useSend(chatId: number, me: number) {
  const qc = useQueryClient()
  return async (body: string, clientId = uuid()) => {
    pendingStore.remove(clientId)
    pendingStore.add({ client_id: clientId, chat_id: chatId, body, status: 'sending' })
    try {
      const m = await api<Message>(`/api/chats/${chatId}/messages`, {
        method: 'POST',
        body: { body, client_id: clientId },
      })
      storeMessage(qc, m, me, 'created')
    } catch {
      pendingStore.setStatus(clientId, 'failed')
    }
  }
}

export function useEditMessage(me: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: string }) =>
      api<Message>(`/api/messages/${id}`, { method: 'PATCH', body: { body } }),
    onSuccess: (m) => storeMessage(qc, m, me, 'updated'),
  })
}

export function useDeleteMessage(me: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api<Message>(`/api/messages/${id}`, { method: 'DELETE' }),
    onSuccess: (m) => storeMessage(qc, m, me, 'deleted'),
  })
}

export function useMarkRead() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ chatId, seq }: { chatId: number; seq: number }) =>
      api<{ last_read_seq: number }>(`/api/chats/${chatId}/read`, { method: 'POST', body: { seq } }),
    onMutate: ({ chatId, seq }) => qc.setQueryData<Chat[]>(keys.chats, (l) => applyReadToList(l, chatId, seq)),
  })
}

/** Group and member changes all return the updated chat. */
export function useChatMutation<V>(chatId: number, fn: (vars: V) => Promise<Chat | void>) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSuccess: (chat) => {
      if (chat) qc.setQueryData(keys.chat(chatId), chat)
      void qc.invalidateQueries({ queryKey: keys.chats })
    },
  })
}
