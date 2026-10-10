// Breakpoint test (Part 2.4): add users until it breaks — how many users does
// one server handle, and what gives out first?
//   k6 run -e BASE_URL=https://<domain> -e MAX_USERS=2000 loadtest/breakpoint.js
// Set BCRYPT_COST=4 on the server for this test: setup signs up MAX_USERS users,
// and we're measuring chat capacity here, not logins (that was Part 1.4).
//
// Every delivery is tagged with how many users were online (in steps of STEP),
// and the summary shows p95 delivery per step: the first step over 300 ms is
// the breaking point. Watch the Grafana dashboard while it runs.
import exec from 'k6/execution'
import { createUsers, session } from './lib.js'

const BASE = __ENV.BASE_URL || 'http://localhost:8080'
const MAX = Number(__ENV.MAX_USERS || 2000)
const STEP = Number(__ENV.STEP || 100)
const RAMP_MIN = Number(__ENV.RAMP_MIN || 20)
const HOLD_MIN = 2
const SEND_EVERY_MS = 10_000

const steps = []
for (let n = STEP; n <= MAX; n += STEP) steps.push(n)
const online = () => ({ users: String(Math.min(MAX, Math.ceil(exec.instance.vusActive / STEP) * STEP)) })

export const options = {
  insecureSkipTLSVerify: !!__ENV.INSECURE,
  setupTimeout: '30m',
  scenarios: {
    ramp: { executor: 'ramping-vus', startVUs: 0, stages: [{ duration: `${RAMP_MIN}m`, target: MAX }, { duration: `${HOLD_MIN}m`, target: MAX }], gracefulRampDown: '30s', exec: 'user' },
  },
  // One (never-failing) threshold per step makes k6 report p95 for each.
  thresholds: Object.fromEntries([
    ['http_req_failed', [{ threshold: 'rate<0.05', abortOnFail: true, delayAbortEval: '1m' }]],
    ...steps.map((n) => [`delivery_ms{users:${n}}`, ['p(95)<1e9']]),
  ]),
  summaryTrendStats: ['med', 'p(95)', 'p(99)', 'max', 'count'],
}

export function setup() {
  return { users: createUsers(BASE, MAX, 5), end: Date.now() + (RAMP_MIN + HOLD_MIN) * 60_000 + 60_000 }
}

export function user(data) {
  const me = data.users[(exec.vu.idInTest - 1) % data.users.length]
  session(BASE, me, data.end, SEND_EVERY_MS, online)
}
