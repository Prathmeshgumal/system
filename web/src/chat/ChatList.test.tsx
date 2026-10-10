import { render, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { describe, expect, it } from 'vitest'
import type { Chat, Message, Profile } from '../lib/types'
import { ChatList } from './ChatList'

const bob: Profile = { id: 2, username: 'bob', display_name: 'Bob', bio: '', created_at: '' }

const msg = (over: Partial<Message>): Message => ({
  id: 1,
  chat_id: 1,
  seq: 1,
  sender_id: 2,
  body: 'hi',
  client_id: 'c',
  created_at: new Date().toISOString(),
  edited_at: null,
  deleted_at: null,
  sender: bob,
  ...over,
})

const base: Omit<Chat, 'id' | 'kind'> = {
  name: '',
  created_by: 1,
  created_at: new Date().toISOString(),
  last_seq: 1,
  last_message_at: new Date().toISOString(),
  my_role: 'member',
  last_read_seq: 1,
  unread: 0,
  member_count: 2,
  last_message: null,
}

describe('ChatList', () => {
  it('shows title, last message preview and unread count', () => {
    const chats: Chat[] = [
      { ...base, id: 1, kind: 'direct', other_user: bob, unread: 3, last_message: msg({ body: 'see you at 8' }) },
      { ...base, id: 2, kind: 'group', name: 'Trip', last_message: msg({ chat_id: 2, body: 'tickets?' }) },
      { ...base, id: 3, kind: 'group', name: 'Mine', last_message: msg({ chat_id: 3, sender_id: 1, body: 'done' }) },
      { ...base, id: 4, kind: 'group', name: 'Oops', last_message: msg({ chat_id: 4, deleted_at: 'x', body: '' }) },
    ]
    render(
      <MemoryRouter>
        <ChatList chats={chats} me={1} />
      </MemoryRouter>,
    )
    const items = screen.getAllByRole('link')
    expect(items).toHaveLength(4)

    expect(within(items[0]).getByText('Bob')).toBeInTheDocument()
    expect(within(items[0]).getByText('see you at 8')).toBeInTheDocument()
    expect(within(items[0]).getByLabelText('3 unread')).toHaveTextContent('3')

    expect(within(items[1]).getByText('Bob: tickets?')).toBeInTheDocument() // group: who said it
    expect(within(items[2]).getByText('You: done')).toBeInTheDocument()
    expect(within(items[3]).getByText('Message deleted')).toBeInTheDocument()
    expect(within(items[1]).queryByLabelText(/unread/)).not.toBeInTheDocument()
  })

  it('has a friendly empty state', () => {
    render(
      <MemoryRouter>
        <ChatList chats={[]} me={1} />
      </MemoryRouter>,
    )
    expect(screen.getByText(/No chats yet/)).toBeInTheDocument()
  })
})
