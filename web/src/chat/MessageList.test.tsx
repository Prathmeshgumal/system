import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { Chat, Member, Message } from '../lib/types'
import { MessageList } from './MessageList'

const profile = (id: number) => ({ id, username: `u${id}`, display_name: `User ${id}`, bio: '', created_at: '' })
const member = (id: number, last_read_seq: number): Member => ({
  user_id: id,
  last_read_seq,
  role: 'member',
  joined_at: '',
  user: profile(id),
})
const msg = (seq: number, sender: number, over: Partial<Message> = {}): Message => ({
  id: seq * 10,
  chat_id: 1,
  seq,
  sender_id: sender,
  body: `message ${seq}`,
  client_id: `c${seq}`,
  created_at: '2026-01-01T10:00:00Z',
  edited_at: null,
  deleted_at: null,
  sender: profile(sender),
  reactions: [],
  reply_to: null,
  ...over,
})
const chat = (kind: 'direct' | 'group', members: Member[]) => ({ id: 1, kind, name: 'g', members }) as Chat

function renderList(c: Chat, messages: Message[], onReact = vi.fn()) {
  render(
    <MessageList
      chat={c}
      messages={messages}
      pending={[]}
      me={1}
      hasOlder={false}
      loadingOlder={false}
      loadOlder={() => {}}
      onEdit={() => {}}
      onDelete={() => {}}
      onRetry={() => {}}
      onReply={() => {}}
      onReact={onReact}
    />,
  )
  return onReact
}

describe('MessageList', () => {
  it('1:1: ✓ until the other person reads, then ✓✓', () => {
    renderList(chat('direct', [member(1, 2), member(2, 1)]), [msg(1, 1), msg(2, 1)])
    const items = screen.getAllByRole('listitem')
    expect(within(items[0]).getByLabelText('Seen')).toBeInTheDocument()
    expect(within(items[1]).getByLabelText('Sent')).toBeInTheDocument()
  })

  it('groups: "Seen by N" only under my newest message', () => {
    renderList(chat('group', [member(1, 3), member(2, 3), member(3, 3), member(4, 1)]), [msg(1, 1), msg(2, 1), msg(3, 2)])
    expect(screen.getAllByText(/Seen by/)).toHaveLength(1)
    expect(within(screen.getAllByRole('listitem')[1]).getByText('Seen by 2')).toBeInTheDocument()
  })

  it('shows the quote of a replied-to message, and "deleted" if it was deleted', () => {
    renderList(chat('direct', [member(1, 0), member(2, 0)]), [
      msg(1, 2, { reply_to: { id: 5, seq: 0, sender: profile(1), body: 'lunch?', deleted: false } }),
      msg(2, 2, { reply_to: { id: 6, seq: 0, sender: profile(1), body: '', deleted: true } }),
    ])
    const quotes = screen.getAllByLabelText('Replied-to message')
    expect(quotes[0]).toHaveTextContent('User 1')
    expect(quotes[0]).toHaveTextContent('lunch?')
    expect(quotes[1]).toHaveTextContent('Message deleted')
  })

  it('reaction chips show counts and toggle MY reaction', async () => {
    const onReact = renderList(chat('direct', [member(1, 0), member(2, 0)]), [
      msg(1, 2, { reactions: [{ emoji: '👍', user_ids: [1, 2] }, { emoji: '😂', user_ids: [2] }] }),
    ])
    const thumbs = screen.getByRole('button', { name: '👍 2' })
    expect(thumbs).toHaveAttribute('aria-pressed', 'true')
    await userEvent.click(thumbs)
    expect(onReact).toHaveBeenLastCalledWith(expect.objectContaining({ seq: 1 }), '👍', false) // remove mine
    await userEvent.click(screen.getByRole('button', { name: '😂 1' }))
    expect(onReact).toHaveBeenLastCalledWith(expect.objectContaining({ seq: 1 }), '😂', true) // add mine
  })
})
