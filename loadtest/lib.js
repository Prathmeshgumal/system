// What one simulated user does, shared by soak.js and breakpoint.js (Part 2.4).
//
// Like the real app: log in, get a ticket, open the WebSocket, and while
// connected: send a typing frame, then a message, every `sendEveryMs`; mark the
// chat read when messages arrive (at most every 5 s, like a visible tab).
// Each message carries its send time, so receivers measure delivery latency.
import http from 'k6/http'
import { check } from 'k6'
import { Counter, Trend } from 'k6/metrics'
import { clearInterval, setInterval, setTimeout } from 'k6/timers'
import { WebSocket } from 'k6/websockets'

export const PASSWORD = 'loadtest-password'

export const delivery = new Trend('delivery_ms', true)
export const sent = new Counter('messages_sent')
export const received = new Counter('messages_received')
export const readsSent = new Counter('reads_sent')
export const eventsReceived = new Counter('ws_events_received')
export const wsErrors = new Counter('ws_errors')

export const json = { headers: { 'Content-Type': 'application/json' } }
export const auth = (token) => ({ headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` } })

/** Signs up `count` users in batches and puts them in groups of `groupSize`. */
export function createUsers(base, count, groupSize) {
  const run = Date.now().toString(36)
  const users = []
  for (let i = 0; i < count; i += 20) {
    const reqs = []
    for (let j = i; j < Math.min(i + 20, count); j++) {
      reqs.push(['POST', `${base}/api/auth/signup`, JSON.stringify({
        email: `lt${j}_${run}@example.com`, username: `lt${j}_${run}`, display_name: `Load ${j}`, password: PASSWORD,
      }), { ...json, tags: { name: 'signup' } }])
    }
    for (const res of http.batch(reqs)) {
      check(res, { 'signup 201': (r) => r.status === 201 })
      const body = res.json()
      users.push({ id: body.user.id, email: body.user.email, token: body.access_token })
    }
  }
  // Group g = users g, g+G, g+2G, … (G = number of groups): mixes early and late starters.
  const groups = Math.ceil(count / groupSize)
  for (let g = 0; g < groups; g++) {
    const members = []
    for (let k = 1; k < groupSize && g + k * groups < count; k++) members.push(users[g + k * groups].id)
    const res = http.post(`${base}/api/chats/group`, JSON.stringify({ name: `load ${g}`, member_ids: members }),
      { ...auth(users[g].token), tags: { name: 'create_group' } })
    check(res, { 'group 201': (r) => r.status === 201 })
    const chatId = res.json().id
    users[g].chatId = chatId
    for (let k = 1; k < groupSize && g + k * groups < count; k++) users[g + k * groups].chatId = chatId
  }
  return users.map(({ id, email, chatId }) => ({ id, email, chatId }))
}

/**
 * One user's session until `untilMs` (epoch ms). `tag` (optional) labels the
 * delivery samples, e.g. with how many users were online when it was sent.
 */
export function session(base, me, untilMs, sendEveryMs, tag = () => ({})) {
  const wsBase = base.replace(/^http/, 'ws')
  const login = http.post(`${base}/api/auth/login`, JSON.stringify({ email: me.email, password: PASSWORD }),
    { ...json, tags: { name: 'login' } })
  if (!check(login, { 'login 200': (r) => r.status === 200 })) return
  const token = login.json().access_token
  const ticketRes = http.post(`${base}/api/ws/ticket`, null, { ...auth(token), tags: { name: 'ticket' } })
  if (!check(ticketRes, { 'ticket 200': (r) => r.status === 200 })) return

  const ws = new WebSocket(`${wsBase}/ws?ticket=${ticketRes.json().ticket}`)
  let lastRead = 0
  let pendingRead = 0

  ws.onerror = (e) => {
    wsErrors.add(1)
    console.error(`websocket error: ${e.error}`)
  }

  ws.onmessage = (e) => {
    eventsReceived.add(1)
    const ev = JSON.parse(e.data)
    if (ev.type !== 'message.created' || ev.message.sender_id === me.id) return
    const sentAt = JSON.parse(ev.message.body).t
    received.add(1)
    delivery.add(Date.now() - sentAt, tag())
    // Read receipts, like a visible tab: at most one read call every 5 s.
    pendingRead = Math.max(pendingRead, ev.message.seq)
    if (Date.now() - lastRead > 5000) {
      lastRead = Date.now()
      const seq = pendingRead
      http.asyncRequest('POST', `${base}/api/chats/${me.chatId}/read`, JSON.stringify({ seq }),
        { ...auth(token), tags: { name: 'read' } }).then((r) => {
        check(r, { 'read 200': (x) => x.status === 200 })
        readsSent.add(1)
      })
    }
  }

  ws.onopen = () => {
    // Start at a random moment so users don't all send in lockstep.
    setTimeout(() => {
      const timer = setInterval(() => {
        ws.send(JSON.stringify({ type: 'typing', chat_id: me.chatId }))
        const body = JSON.stringify({ t: Date.now() })
        http.asyncRequest('POST', `${base}/api/chats/${me.chatId}/messages`,
          JSON.stringify({ body, client_id: crypto.randomUUID() }), { ...auth(token), tags: { name: 'send' } })
          .then((r) => {
            check(r, { 'send 201': (x) => x.status === 201 })
            sent.add(1)
          })
      }, sendEveryMs)
      setTimeout(() => {
        clearInterval(timer)
        setTimeout(() => ws.close(), 2000) // let the last messages arrive
      }, Math.max(0, untilMs - Date.now()))
    }, Math.random() * sendEveryMs)
  }
}
