import { describe, expect, it } from 'vitest'
import type { Chat, Message } from '../lib/types'
import { allMessages, applyMessageToList, applyReadToList, maxSeq, upsertMessage, type MessagesData } from './cache'

const msg = (seq: number, over: Partial<Message> = {}): Message => ({
  id: seq * 10,
  chat_id: 1,
  seq,
  sender_id: 2,
  body: `m${seq}`,
  client_id: `c${seq}`,
  created_at: `2026-01-01T00:00:${String(seq).padStart(2, '0')}Z`,
  edited_at: null,
  deleted_at: null,
  sender: null,
  ...over,
})

// pages[0] is the newest page; each page is oldest → newest.
const data = (...pages: number[][]): MessagesData => ({
  pages: pages.map((seqs) => ({ messages: seqs.map((s) => msg(s)), has_more: true })),
  pageParams: pages.map(() => undefined),
})

const seqs = (d: MessagesData | undefined) => allMessages(d).map((m) => m.seq)

describe('message cache', () => {
  it('flattens pages into one oldest → newest list', () => {
    expect(seqs(data([4, 5, 6], [1, 2, 3]))).toEqual([1, 2, 3, 4, 5, 6])
  })

  it('appends a new message to the newest page', () => {
    expect(seqs(upsertMessage(data([4, 5]), msg(6)))).toEqual([4, 5, 6])
  })

  it('ignores a duplicate (event and catch-up both carry seq 5)', () => {
    expect(seqs(upsertMessage(data([4, 5]), msg(5)))).toEqual([4, 5])
  })

  it('keeps order when events arrive out of order', () => {
    const d = upsertMessage(upsertMessage(data([4]), msg(6)), msg(5))
    expect(seqs(d)).toEqual([4, 5, 6])
  })

  it('replaces an edited or deleted message in place, on any page', () => {
    const d = upsertMessage(data([4, 5], [1, 2, 3]), msg(2, { body: 'edited', edited_at: 'now' }))
    expect(allMessages(d).find((m) => m.seq === 2)?.body).toBe('edited')
    expect(seqs(d)).toEqual([1, 2, 3, 4, 5])
  })

  it("doesn't add messages older than what's loaded (scrolling will fetch them)", () => {
    expect(seqs(upsertMessage(data([4, 5]), msg(1)))).toEqual([4, 5])
  })

  it('leaves an unloaded chat alone', () => {
    expect(upsertMessage(undefined, msg(1))).toBeUndefined()
  })

  it('maxSeq is the newest loaded seq', () => {
    expect(maxSeq(data([4, 5], [1]))).toBe(5)
    expect(maxSeq(data([]))).toBe(0)
  })
})

const chat = (id: number, over: Partial<Chat> = {}): Chat => ({
  id,
  kind: 'direct',
  name: '',
  created_by: 1,
  created_at: '2026-01-01T00:00:00Z',
  last_seq: 3,
  last_message_at: '2026-01-01T00:00:00Z',
  my_role: 'member',
  last_read_seq: 3,
  unread: 0,
  member_count: 2,
  last_message: null,
  ...over,
})

describe('chat list cache', () => {
  const ME = 1

  it("someone else's new message: preview, unread +1, moves to the top", () => {
    const list = [chat(10), chat(20)]
    const { chats, found } = applyMessageToList(list, msg(4, { chat_id: 20, sender_id: 2 }), ME, 'created')
    expect(found).toBe(true)
    expect(chats!.map((c) => c.id)).toEqual([20, 10])
    expect(chats![0]).toMatchObject({ last_seq: 4, unread: 1 })
    expect(chats![0].last_message?.body).toBe('m4')
  })

  it('my own message is never unread', () => {
    const { chats } = applyMessageToList([chat(20)], msg(4, { chat_id: 20, sender_id: ME }), ME, 'created')
    expect(chats![0]).toMatchObject({ last_seq: 4, last_read_seq: 4, unread: 0 })
  })

  it('a duplicate event does not count twice', () => {
    const once = applyMessageToList([chat(20)], msg(4, { chat_id: 20 }), ME, 'created').chats
    const twice = applyMessageToList(once, msg(4, { chat_id: 20 }), ME, 'created').chats
    expect(twice![0].unread).toBe(1)
  })

  it('an edit of the last message updates the preview but not the order or unread', () => {
    const list = [chat(10), chat(20, { last_message: msg(3, { chat_id: 20 }) })]
    const { chats } = applyMessageToList(list, msg(3, { chat_id: 20, body: 'fixed' }), ME, 'updated')
    expect(chats!.map((c) => c.id)).toEqual([10, 20])
    expect(chats![1].last_message?.body).toBe('fixed')
    expect(chats![1].unread).toBe(0)
  })

  it('reports unknown chats so the caller can refetch', () => {
    expect(applyMessageToList([chat(10)], msg(1, { chat_id: 99 }), ME, 'created').found).toBe(false)
  })

  it('read marker clears unread and never moves backwards', () => {
    let chats = applyReadToList([chat(20, { last_seq: 9, last_read_seq: 2, unread: 7 })], 20, 6)
    expect(chats![0]).toMatchObject({ last_read_seq: 6, unread: 3 })
    chats = applyReadToList(chats, 20, 4)
    expect(chats![0]).toMatchObject({ last_read_seq: 6, unread: 3 })
  })
})
