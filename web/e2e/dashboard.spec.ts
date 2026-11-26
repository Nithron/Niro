import { expect, test } from '@playwright/test'
import { fakeState, mockApi, type FakeState } from './api'

let state: FakeState

test.beforeEach(async ({ page }) => {
  state = fakeState()
  await mockApi(page, state)
})

test('shows the Discord login without a session', async ({ page }) => {
  state.loggedIn = false
  await page.goto('/')
  await expect(page.getByRole('link', { name: 'Log in with Discord' })).toBeVisible()
})

test('opens a server and turns a module on', async ({ page }) => {
  await page.goto('/')
  await page.getByRole('link', { name: 'Manage' }).click()
  const toggle = page.getByRole('switch', { name: 'Logging' })
  await expect(toggle).toHaveAttribute('aria-checked', 'false')
  await toggle.click()
  await expect(toggle).toHaveAttribute('aria-checked', 'true')
  await expect(page.getByText('Saved')).toBeVisible()
  expect(state.modules.find((m) => m.id === 'logging')?.enabled).toBe(true)
  // The change survives a reload.
  await page.reload()
  await expect(page.getByRole('switch', { name: 'Logging' })).toHaveAttribute('aria-checked', 'true')
})

test('saves Guardian settings', async ({ page }) => {
  await page.goto('/guilds/100/guardian')
  await page.getByLabel('Mode', { exact: true }).selectOption('monitor')
  await page.getByLabel('Alert channel', { exact: true }).selectOption('500')
  await page.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByText('Saved')).toBeVisible()
  expect(state.guardian).toMatchObject({ mode: 'monitor', alertChannelId: '500' })
})

test('routes the server log and saves it', async ({ page }) => {
  await page.goto('/guilds/100/logging')
  await expect(page.getByText('No log channel is set, so nothing is logged yet.')).toBeVisible()
  await page.getByLabel('Default log channel').selectOption('500')
  await page.getByRole('switch', { name: 'Voice joins' }).click()
  await page.getByRole('switch', { name: "Don't log message text" }).click()
  await page.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByText('Saved')).toBeVisible()
  expect(state.logging).toMatchObject({ defaultChannelId: '500', noMessageContent: true })
  expect((state.logging.events as Record<string, boolean>).voice_join).toBe(true)
  await page.reload()
  await expect(page.getByLabel('Default log channel')).toHaveValue('500')
})
