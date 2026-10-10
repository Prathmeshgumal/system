import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { backoffDelay, RealtimeSocket } from './socket'

describe('backoffDelay', () => {
  it('doubles from 0.5 s up to a 30 s cap, with jitter between 50% and 100%', () => {
    const max = (n: number) => backoffDelay(n, () => 1)
    const min = (n: number) => backoffDelay(n, () => 0)
    expect([0, 1, 2, 3].map(max)).toEqual([500, 1000, 2000, 4000])
    expect(max(20)).toBe(30_000)
    expect(min(3)).toBe(2000)
  })
})

// A stand-in for the browser's WebSocket that tests drive by hand.
class FakeWS {
  static all: FakeWS[] = []
  url: string
  onopen: (() => void) | null = null
  onclose: (() => void) | null = null
  onmessage: ((e: { data: string }) => void) | null = null
  onerror: (() => void) | null = null
  closed = false
  constructor(url: string) {
    this.url = url
    FakeWS.all.push(this)
  }
  close() {
    this.closed = true
  }
  open() {
    this.onopen?.()
  }
  drop() {
    this.onclose?.()
  }
  push(ev: unknown) {
    this.onmessage?.({ data: JSON.stringify(ev) })
  }
}

describe('RealtimeSocket', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    FakeWS.all = []
  })
  afterEach(() => vi.useRealTimers())

  function make() {
    const events: unknown[] = []
    const opens: number[] = []
    const statuses: string[] = []
    const s = new RealtimeSocket({
      getTicket: async () => 'tkt',
      url: (t) => `ws://test/ws?ticket=${t}`,
      onEvent: (e) => events.push(e),
      onOpen: () => opens.push(Date.now()),
      onStatus: (st) => statuses.push(st),
      WebSocketImpl: FakeWS as unknown as typeof WebSocket,
      random: () => 1,
    })
    return { s, events, opens, statuses }
  }

  it('connects with a fresh ticket and delivers events', async () => {
    const { s, events, opens } = make()
    s.start()
    await vi.runAllTicks()
    await Promise.resolve()
    expect(FakeWS.all[0].url).toBe('ws://test/ws?ticket=tkt')
    FakeWS.all[0].open()
    FakeWS.all[0].push({ type: 'chat.read', chat_id: 1, seq: 2 })
    expect(opens).toHaveLength(1)
    expect(events).toEqual([{ type: 'chat.read', chat_id: 1, seq: 2 }])
    s.stop()
  })

  it('reconnects after a drop with growing delays, and resets after success', async () => {
    const { s, opens } = make()
    s.start()
    await vi.advanceTimersByTimeAsync(0)
    FakeWS.all[0].open()

    FakeWS.all[0].drop()
    await vi.advanceTimersByTimeAsync(499)
    expect(FakeWS.all).toHaveLength(1) // still waiting
    await vi.advanceTimersByTimeAsync(1)
    expect(FakeWS.all).toHaveLength(2) // attempt 1 after 500 ms

    FakeWS.all[1].drop() // failed before opening
    await vi.advanceTimersByTimeAsync(1000)
    expect(FakeWS.all).toHaveLength(3) // attempt 2 after 1000 ms

    FakeWS.all[2].open()
    expect(opens).toHaveLength(2) // onOpen runs on every (re)connect → catch-up

    FakeWS.all[2].drop()
    await vi.advanceTimersByTimeAsync(500)
    expect(FakeWS.all).toHaveLength(4) // back to 500 ms after a good connection
    s.stop()
  })

  it('stop() closes the socket and never reconnects', async () => {
    const { s } = make()
    s.start()
    await vi.advanceTimersByTimeAsync(0)
    FakeWS.all[0].open()
    s.stop()
    expect(FakeWS.all[0].closed).toBe(true)
    FakeWS.all[0].drop()
    await vi.advanceTimersByTimeAsync(60_000)
    expect(FakeWS.all).toHaveLength(1)
  })

  it('retries when getting a ticket fails (e.g. server down)', async () => {
    let calls = 0
    const s = new RealtimeSocket({
      getTicket: async () => {
        calls++
        if (calls === 1) throw new Error('down')
        return 't'
      },
      url: (t) => t,
      onEvent: () => {},
      WebSocketImpl: FakeWS as unknown as typeof WebSocket,
      random: () => 1,
    })
    s.start()
    await vi.advanceTimersByTimeAsync(0)
    expect(FakeWS.all).toHaveLength(0)
    await vi.advanceTimersByTimeAsync(500)
    expect(FakeWS.all).toHaveLength(1)
    s.stop()
  })
})
