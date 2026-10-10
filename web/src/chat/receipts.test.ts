import { describe, expect, it } from 'vitest'
import type { Chat, Member } from '../lib/types'
import { applyMemberRead, directReceipt, seenBy } from './receipts'

const member = (user_id: number, last_read_seq: number): Member => ({
  user_id,
  last_read_seq,
  role: 'member',
  joined_at: '',
  user: { id: user_id, username: `u${user_id}`, display_name: `U${user_id}`, bio: '', created_at: '' },
})

const chat = (members: Member[]): Chat =>
  ({ id: 1, kind: members.length > 2 ? 'group' : 'direct', members }) as Chat

describe('read receipts', () => {
  it('1:1: seen once the other person has read that far', () => {
    const c = chat([member(1, 9), member(2, 4)])
    expect(directReceipt(c, 4, 1)).toBe('seen')
    expect(directReceipt(c, 5, 1)).toBe('sent')
  })

  it('groups: counts other members who have read it, never me', () => {
    const c = chat([member(1, 9), member(2, 5), member(3, 7), member(4, 2)])
    expect(seenBy(c, 5, 1)).toBe(2)
    expect(seenBy(c, 8, 1)).toBe(0)
  })

  it('a read event moves a member forward, never back', () => {
    let c = applyMemberRead(chat([member(1, 0), member(2, 3)]), 2, 7)
    expect(c!.members![1].last_read_seq).toBe(7)
    c = applyMemberRead(c, 2, 5)
    expect(c!.members![1].last_read_seq).toBe(7)
  })
})
