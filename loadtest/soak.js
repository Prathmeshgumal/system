// Soak test (Part 2.4, v0.2 "done when"): 100 users chatting for 30 minutes.
//   k6 run -e BASE_URL=https://<domain> loadtest/soak.js            (make loadtest-soak)
//   -e DURATION_MIN=5 for a short run
// Pass: delivery p95 < 300 ms, no errors.
import exec from 'k6/execution'
import { createUsers, session } from './lib.js'

const BASE = __ENV.BASE_URL || 'http://localhost:8080'
const USERS = Number(__ENV.USERS || 100)
const DURATION_MIN = Number(__ENV.DURATION_MIN || 30)
const SEND_EVERY_MS = 10_000 // 3× the design estimate of one message per 30 s

export const options = {
  insecureSkipTLSVerify: !!__ENV.INSECURE,
  setupTimeout: '10m',
  scenarios: {
    // Users arrive over the first minute (a real morning, not a stampede).
    soak: { executor: 'ramping-vus', startVUs: 0, stages: [{ duration: '1m', target: USERS }, { duration: `${DURATION_MIN}m`, target: USERS }], gracefulRampDown: '30s', exec: 'user' },
  },
  thresholds: {
    delivery_ms: ['p(95)<300'],
    http_req_failed: ['rate<0.001'],
    checks: ['rate>0.999'],
    ws_errors: ['count==0'],
  },
}

export function setup() {
  return { users: createUsers(BASE, USERS, 5), end: Date.now() + (DURATION_MIN + 1) * 60_000 }
}

export function user(data) {
  const me = data.users[(exec.vu.idInTest - 1) % data.users.length]
  session(BASE, me, data.end, SEND_EVERY_MS)
}
