import { expect, test, type WebSocketRoute } from '@playwright/test'
import { run, send, signUp, startChat } from './helpers.ts'

test('a reload keeps you logged in (refresh cookie → new access token)', async ({ browser }) => {
  const page = await signUp(browser, 'gina')
  await page.reload()
  await expect(page.getByRole('status')).toHaveText('Connected')
  await expect(page.getByRole('link', { name: /gina/ })).toBeVisible() // the profile link in the header

  await page.getByRole('button', { name: 'Log out' }).click()
  await expect(page.getByRole('heading', { name: 'Log in' })).toBeVisible()
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Log in' })).toBeVisible() // the session is really gone
})

test('two users chat live: send, unread badge, reply, edit, delete', async ({ browser }) => {
  const alice = await signUp(browser, 'alice')
  const bob = await signUp(browser, 'bob')

  await startChat(alice, `bob_${run}`)
  await send(alice, 'hi bob!')

  // Bob never reloads: the chat and its unread badge arrive over the WebSocket.
  const chatLink = bob.getByRole('link', { name: new RegExp(`alice ${run}`) })
  await expect(chatLink.getByLabel('1 unread')).toBeVisible()
  await chatLink.click()
  await expect(bob.getByRole('list', { name: 'Messages' })).toContainText('hi bob!')
  await expect(chatLink.getByLabel(/unread/)).toHaveCount(0) // opening it marks it read

  // Live delivery, measured from the sender's Enter to the text on the other screen.
  const started = Date.now()
  await send(bob, 'hey alice')
  await expect(alice.getByRole('list', { name: 'Messages' })).toContainText('hey alice')
  const ms = Date.now() - started
  console.log(`delivery seen in the other browser after ${ms} ms`)
  expect(ms).toBeLessThan(1000) // generous for CI; locally this is tens of ms

  // Edit and delete show up live on the other side.
  const mine = alice.getByRole('listitem').filter({ hasText: 'hi bob!' })
  await mine.hover()
  await mine.getByRole('button', { name: 'Edit' }).click()
  await alice.getByLabel('Edit message').fill('hi bob!! (edited)')
  await alice.getByLabel('Edit message').press('Enter')
  await expect(bob.getByRole('list', { name: 'Messages' })).toContainText('hi bob!! (edited)')
  await expect(bob.getByRole('list', { name: 'Messages' })).toContainText('edited ·')

  const edited = alice.getByRole('listitem').filter({ hasText: 'hi bob!! (edited)' })
  await edited.hover()
  await edited.getByRole('button', { name: 'Delete' }).click()
  await expect(bob.getByRole('list', { name: 'Messages' })).toContainText('Message deleted')
})

test('a dropped connection reconnects and catches up on missed messages', async ({ browser }) => {
  // Bob's WebSocket goes through a test-controlled pipe we can cut.
  // (context.setOffline doesn't close an open WebSocket to localhost.)
  let outage = false
  const sockets: WebSocketRoute[] = []
  const bob = await signUp(browser, 'dave', (page) =>
    page.routeWebSocket(/\/ws/, (ws) => {
      if (outage) return ws.close() // the server is unreachable: connection refused
      ws.connectToServer()
      sockets.push(ws)
    }),
  )
  const alice = await signUp(browser, 'carol')
  await startChat(alice, `dave_${run}`)
  await send(alice, 'before the outage')
  await bob.getByRole('link', { name: new RegExp(`carol ${run}`) }).click()
  await expect(bob.getByRole('list', { name: 'Messages' })).toContainText('before the outage')

  // Bob's Wi-Fi drops: the connection dies and reconnect attempts fail.
  outage = true
  await Promise.all(sockets.map((ws) => ws.close()))
  await expect(bob.getByRole('status')).toHaveText('Reconnecting…')

  await send(alice, 'sent while you were away')
  await send(alice, 'and another one')
  await expect(bob.getByRole('list', { name: 'Messages' })).not.toContainText('sent while you were away')

  // Back online: the next attempt connects, then fetches everything after the newest seq Bob had.
  outage = false
  await expect(bob.getByRole('status')).toHaveText('Connected', { timeout: 20_000 })
  const list = bob.getByRole('list', { name: 'Messages' })
  await expect(list).toContainText('sent while you were away')
  await expect(list).toContainText('and another one')
})

test('group: create, members see it live, admin removes a member', async ({ browser }) => {
  const owner = await signUp(browser, 'erin')
  const member = await signUp(browser, 'frank')

  await owner.getByRole('button', { name: 'New group' }).click()
  await owner.getByLabel('Group name').fill(`Team ${run}`)
  await owner.getByLabel('Search users').fill(`frank_${run}`)
  await owner.getByRole('button', { name: new RegExp(`@frank_${run}`) }).click()
  await owner.getByRole('button', { name: 'Create group' }).click()
  await send(owner, 'welcome!')

  const groupLink = member.getByRole('link', { name: new RegExp(`Team ${run}`) })
  await expect(groupLink).toContainText('welcome!')
  await groupLink.click()

  await owner.getByRole('button', { name: 'Group info' }).click()
  await owner.getByRole('button', { name: 'Remove', exact: true }).click()

  await expect(member.getByText("This chat isn't available.")).toBeVisible()
  await expect(groupLink).toHaveCount(0)
})
