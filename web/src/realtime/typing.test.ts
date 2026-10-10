import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { TYPING_TTL_MS, typingStore } from './typing'

describe('typing store', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    typingStore.reset()
  })
  afterEach(() => vi.useRealTimers())

  it('an entry lasts TYPING_TTL_MS, and only for its chat', () => {
    typingStore.add(7, 3)
    expect(typingStore.get(7)).toEqual([3])
    expect(typingStore.get(8)).toEqual([])
    vi.advanceTimersByTime(TYPING_TTL_MS - 1)
    expect(typingStore.get(7)).toEqual([3])
    vi.advanceTimersByTime(1)
    expect(typingStore.get(7)).toEqual([])
  })

  it('another typing event extends it', () => {
    typingStore.add(7, 4)
    vi.advanceTimersByTime(TYPING_TTL_MS - 1000)
    typingStore.add(7, 4)
    vi.advanceTimersByTime(TYPING_TTL_MS - 1000)
    expect(typingStore.get(7)).toEqual([4])
  })

  it('their message clears it at once', () => {
    typingStore.add(7, 5)
    typingStore.clear(7, 5)
    expect(typingStore.get(7)).toEqual([])
  })
})
