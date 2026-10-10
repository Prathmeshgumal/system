// The WebSocket connection: get a ticket, connect, hand events to onEvent,
// and when the connection drops, reconnect with exponential backoff + jitter.
// See knowledge-base/adr/0003-websocket-protocol.md.
import type { ServerEvent } from '../lib/types'

export type SocketStatus = 'connecting' | 'open' | 'reconnecting'

/**
 * Delay before reconnect attempt n (0-based): 0.5 s, 1 s, 2 s, … capped at 30 s,
 * then scaled by a random 50–100%. The randomness ("jitter") matters when the
 * server restarts: without it, every client reconnects at the same instant.
 */
export function backoffDelay(attempt: number, random: () => number = Math.random): number {
  const ceiling = Math.min(30_000, 500 * 2 ** attempt)
  return ceiling / 2 + (ceiling / 2) * random()
}

type Options = {
  getTicket: () => Promise<string>
  url: (ticket: string) => string
  onEvent: (e: ServerEvent) => void
  /** Runs after every successful (re)connect — the moment to catch up on missed messages. */
  onOpen?: () => void
  onStatus?: (s: SocketStatus) => void
  WebSocketImpl?: typeof WebSocket
  random?: () => number
}

export class RealtimeSocket {
  private opts: Options
  private ws: WebSocket | null = null
  private attempt = 0
  private timer: ReturnType<typeof setTimeout> | undefined
  private stopped = true

  constructor(opts: Options) {
    this.opts = opts
  }

  start() {
    this.stopped = false
    void this.connect()
  }

  stop() {
    this.stopped = true
    clearTimeout(this.timer)
    this.ws?.close()
    this.ws = null
  }

  private async connect() {
    this.opts.onStatus?.(this.attempt === 0 ? 'connecting' : 'reconnecting')
    let ticket: string
    try {
      ticket = await this.opts.getTicket()
    } catch {
      this.retry()
      return
    }
    if (this.stopped) return

    const WS = this.opts.WebSocketImpl ?? WebSocket
    const ws = new WS(this.opts.url(ticket))
    this.ws = ws
    ws.onopen = () => {
      this.attempt = 0
      this.opts.onStatus?.('open')
      this.opts.onOpen?.()
    }
    ws.onmessage = (e: MessageEvent) => {
      try {
        this.opts.onEvent(JSON.parse(e.data as string) as ServerEvent)
      } catch (err) {
        console.error('bad event from server', err)
      }
    }
    ws.onclose = () => {
      if (this.ws === ws) this.ws = null
      this.retry()
    }
  }

  private retry() {
    if (this.stopped) return
    this.opts.onStatus?.('reconnecting')
    const delay = backoffDelay(this.attempt++, this.opts.random)
    this.timer = setTimeout(() => void this.connect(), delay)
  }
}
