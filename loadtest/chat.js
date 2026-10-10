// Part 1.4 — 100 users chatting at once. See knowledge-base/phases/phase-1/design.md, section 6.
//
//   make loadtest                      (API must be running: make dev or make api)
//   k6 run -e BASE_URL=http://localhost:8080 loadtest/chat.js
//
// Shape of the test:
//   setup     sign up 100 users, put them in 20 groups of 5
//   t = 0s    users 1–50 log in, open a WebSocket and start chatting
//   t = 30s   users 51–100 log in AT ONCE (a login burst) while the first 50 chat
//   t = 120s  everyone stops
// Every user sends a message every 3 s (≈10× the design estimate). Each message
// carries its send time, so receivers can measure delivery latency end to end.
import http from 'k6/http'
import { check } from 'k6'
import exec from 'k6/execution'
import { Counter, Trend } from 'k6/metrics'
import { setInterval, clearInterval, setTimeout } from 'k6/timers'
import { WebSocket } from 'k6/websockets'

const BASE = __ENV.BASE_URL || 'http://localhost:8080'
const WS_BASE = BASE.replace(/^http/, 'ws')
const USERS = 100
const GROUP_SIZE = 5
const SEND_EVERY_MS = 3000
const LATE_START_S = 30
const END_S = 120
const PASSWORD = 'loadtest-password'

const delivery = new Trend('delivery_ms', true) // sender's POST → receiver's WebSocket event
const deliveryDuringBurst = new Trend('delivery_during_login_burst_ms', true)
const loginTime = new Trend('login_ms', true)
const sent = new Counter('messages_sent')
const received = new Counter('messages_received')
const wsErrors = new Counter('ws_errors')

export const options = {
  setupTimeout: '180s',
  scenarios: {
    early: { executor: 'per-vu-iterations', vus: 50, iterations: 1, maxDuration: '3m', exec: 'chat' },
    late: { executor: 'per-vu-iterations', vus: 50, iterations: 1, maxDuration: '3m', exec: 'chat', startTime: `${LATE_START_S}s` },
  },
  thresholds: {
    http_req_failed: ['rate==0'],
    checks: ['rate==1'],
    ws_errors: ['count==0'],
    delivery_ms: ['p(95)<200'],
    'http_req_duration{name:send}': ['p(95)<200'],
  },
}

const json = { headers: { 'Content-Type': 'application/json' } }
const auth = (token) => ({ headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` } })

export function setup() {
  const run = Date.now().toString(36)
  const users = []
  // Sign up in batches of 10 (each signup is a ~250 ms bcrypt hash on the server).
  for (let i = 0; i < USERS; i += 10) {
    const reqs = []
    for (let j = i; j < i + 10; j++) {
      reqs.push(['POST', `${BASE}/api/auth/signup`, JSON.stringify({
        email: `lt${j}_${run}@example.com`, username: `lt${j}_${run}`, display_name: `Load ${j}`, password: PASSWORD,
      }), { ...json, tags: { name: 'signup' } }])
    }
    for (const res of http.batch(reqs)) {
      check(res, { 'signup 201': (r) => r.status === 201 })
      const body = res.json()
      users.push({ id: body.user.id, email: body.user.email, token: body.access_token })
    }
  }
  // Group g = users g, g+20, g+40, g+60, g+80: every group mixes early and late users.
  const groups = USERS / GROUP_SIZE
  for (let g = 0; g < groups; g++) {
    const members = []
    for (let k = 1; k < GROUP_SIZE; k++) members.push(users[g + k * groups].id)
    const res = http.post(`${BASE}/api/chats/group`, JSON.stringify({ name: `load ${g}`, member_ids: members }),
      { ...auth(users[g].token), tags: { name: 'create_group' } })
    check(res, { 'group 201': (r) => r.status === 201 })
    const chatId = res.json().id
    users[g].chatId = chatId
    for (let k = 1; k < GROUP_SIZE; k++) users[g + k * groups].chatId = chatId
  }
  return { users: users.map(({ email, chatId, id }) => ({ email, chatId, id })), start: Date.now() + 1000 }
}

export function chat(data) {
  const me = data.users[exec.vu.idInTest - 1]
  const endAt = data.start + END_S * 1000
  const burst = [data.start + LATE_START_S * 1000, data.start + (LATE_START_S + 15) * 1000]

  const t0 = Date.now()
  const login = http.post(`${BASE}/api/auth/login`, JSON.stringify({ email: me.email, password: PASSWORD }),
    { ...json, tags: { name: 'login' } })
  loginTime.add(Date.now() - t0)
  if (!check(login, { 'login 200': (r) => r.status === 200 })) return
  const token = login.json().access_token

  const ticket = http.post(`${BASE}/api/ws/ticket`, null, { ...auth(token), tags: { name: 'ticket' } }).json().ticket
  const ws = new WebSocket(`${WS_BASE}/ws?ticket=${ticket}`)

  ws.onerror = (e) => {
    wsErrors.add(1)
    console.error(`vu ${exec.vu.idInTest}: websocket error: ${e.error}`)
  }

  ws.onmessage = (e) => {
    const ev = JSON.parse(e.data)
    if (ev.type !== 'message.created' || ev.message.sender_id === me.id) return
    const sentAt = JSON.parse(ev.message.body).t
    const now = Date.now()
    received.add(1)
    delivery.add(now - sentAt)
    if (sentAt >= burst[0] && sentAt < burst[1]) deliveryDuringBurst.add(now - sentAt)
  }

  ws.onopen = () => {
    const timer = setInterval(() => {
      const body = JSON.stringify({ t: Date.now(), from: exec.vu.idInTest })
      http.asyncRequest('POST', `${BASE}/api/chats/${me.chatId}/messages`,
        JSON.stringify({ body, client_id: crypto.randomUUID() }), { ...auth(token), tags: { name: 'send' } })
        .then((res) => {
          check(res, { 'send 201': (r) => r.status === 201 })
          sent.add(1)
        })
    }, SEND_EVERY_MS)

    setTimeout(() => {
      clearInterval(timer)
      // Linger 2 s so the last messages from others still arrive.
      setTimeout(() => ws.close(), 2000)
    }, Math.max(0, endAt - Date.now()))
  }
}
