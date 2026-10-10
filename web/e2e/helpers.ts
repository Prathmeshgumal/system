import { expect, type Browser, type Page } from '@playwright/test'

// Unique per run, so the tests can run against your normal dev database.
export const run = Date.now().toString(36)

export async function signUp(browser: Browser, name: string, before?: (p: Page) => Promise<void>): Promise<Page> {
  const page = await (await browser.newContext()).newPage()
  await before?.(page)
  await page.goto('/signup')
  await page.getByLabel('Email').fill(`${name}_${run}@example.com`)
  await page.getByLabel('Username').fill(`${name}_${run}`)
  await page.getByLabel('Display name').fill(`${name} ${run}`)
  await page.getByLabel('Password').fill('password123')
  await page.getByRole('button', { name: 'Sign up' }).click()
  await expect(page.getByRole('status')).toHaveText('Connected') // WebSocket is up
  return page
}

export async function startChat(from: Page, username: string) {
  await from.getByRole('button', { name: 'New chat' }).click()
  await from.getByLabel('Search users').fill(username)
  await from.getByRole('button', { name: new RegExp(`@${username}`) }).click()
}

export async function send(page: Page, text: string) {
  await page.getByLabel('Message', { exact: true }).fill(text)
  await page.getByLabel('Message', { exact: true }).press('Enter')
}

export const messages = (page: Page) => page.getByRole('list', { name: 'Messages' })
