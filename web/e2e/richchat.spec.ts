import { expect, test } from '@playwright/test'
import { messages, run, send, signUp, startChat } from './helpers.ts'

test('typing, read receipts, reactions and replies, live in both browsers', async ({ browser }) => {
  const alice = await signUp(browser, 'hana')
  const bob = await signUp(browser, 'ivan')
  await startChat(alice, `ivan_${run}`)
  await send(alice, 'anyone up for tennis?')

  // Receipt: ✓ while Bob hasn't opened the chat…
  const myMsg = messages(alice).getByRole('listitem').filter({ hasText: 'anyone up for tennis?' })
  await expect(myMsg.getByLabel('Sent')).toBeVisible()
  await bob.getByRole('link', { name: new RegExp(`hana ${run}`) }).click()
  // …✓✓ as soon as he has.
  await expect(myMsg.getByLabel('Seen')).toBeVisible()

  // Typing: Bob types, Alice sees it; it clears when his message arrives.
  await bob.getByLabel('Message', { exact: true }).pressSequentially('sure', { delay: 50 })
  await expect(alice.getByText(`ivan ${run} is typing…`)).toBeVisible()
  await bob.getByLabel('Message', { exact: true }).press('Enter')
  await expect(messages(alice)).toContainText('sure')
  await expect(alice.getByText(/is typing/)).toHaveCount(0)

  // Reaction: Bob reacts 👍 to Alice's message; Alice sees the count.
  const onBob = messages(bob).getByRole('listitem').filter({ hasText: 'anyone up for tennis?' })
  await onBob.hover()
  await onBob.getByRole('button', { name: 'React' }).click()
  await bob.getByRole('group', { name: 'Pick a reaction' }).getByRole('button', { name: '👍' }).click()
  await expect(myMsg.getByRole('button', { name: '👍 1' })).toBeVisible()

  // Reply: Alice replies to Bob's "sure"; Bob sees the quote.
  const bobsMsg = messages(alice).getByRole('listitem').filter({ hasText: 'sure' })
  await bobsMsg.hover()
  await bobsMsg.getByRole('button', { name: 'Reply' }).click()
  await expect(alice.getByText(`Replying to ivan ${run}`)).toBeVisible()
  await send(alice, 'great, 6pm')
  const reply = messages(bob).getByRole('listitem').filter({ hasText: 'great, 6pm' })
  await expect(reply.getByLabel('Replied-to message')).toContainText('sure')
})

test('dark mode toggle sticks across reloads', async ({ browser }) => {
  const page = await signUp(browser, 'jade')
  const html = page.locator('html')
  await page.getByRole('button', { name: /Auto/ }).click() // → light
  await page.getByRole('button', { name: /Light/ }).click() // → dark
  await expect(html).toHaveClass(/dark/)
  await page.reload()
  await expect(html).toHaveClass(/dark/) // applied before React loads: no white flash
})
