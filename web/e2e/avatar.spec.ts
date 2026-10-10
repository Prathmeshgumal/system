import { expect, test } from '@playwright/test'
import { run, send, signUp, startChat } from './helpers.ts'

// A 2×2 red PNG — enough for the browser to decode, crop and re-encode.
const png = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAIAAAD91JpzAAAAFklEQVR4nGP8z8DAwMDAxMDAwMDAAAANHQEDasKb6QAAAABJRU5ErkJggg==',
  'base64',
)

test('upload a profile picture; the other person sees it', async ({ browser }) => {
  const kim = await signUp(browser, 'kim')
  const leo = await signUp(browser, 'leo')
  await startChat(leo, `kim_${run}`)
  await send(leo, 'hi kim')

  await kim.goto('/profile')
  await kim.getByLabel('Profile picture').setInputFiles({ name: 'me.png', mimeType: 'image/png', buffer: png })
  await expect(kim.getByRole('button', { name: 'Remove picture' })).toBeVisible()

  // The picture is served by the API from storage, cached forever.
  const src = await kim.locator('img[src^="/media/avatars/"]').first().getAttribute('src')
  const res = await kim.request.get(src!)
  expect(res.status()).toBe(200)
  expect(res.headers()['content-type']).toBe('image/jpeg') // resized to JPEG in the browser
  expect(res.headers()['cache-control']).toContain('immutable')

  // Leo sees it next to the chat (profile changes aren't pushed live; a reload fetches them).
  await leo.reload()
  await expect(leo.getByRole('link', { name: new RegExp(`kim ${run}`) }).locator('img')).toHaveAttribute('src', src!)
})
