import { expect, test } from '@playwright/test'
import { fakeState, mockApi, type FakeState } from './api'

let state: FakeState

test.beforeEach(async ({ page }) => {
  state = fakeState()
  await mockApi(page, state)
})

test('shows the Terms and Privacy Policy without a session', async ({ page }) => {
  await page.goto('/privacy')
  await expect(page.getByRole('heading', { level: 1, name: 'Privacy Policy' })).toBeVisible()
  await expect(page.getByRole('table').first()).toBeVisible()
  await page.screenshot({ path: 'test-results/privacy.png', fullPage: false })
  await page.getByRole('link', { name: 'Terms', exact: true }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'Terms of Service' })).toBeVisible()
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

test('builds and posts a role panel', async ({ page }) => {
  await page.goto('/guilds/100/roles')
  await page.getByRole('button', { name: 'New panel' }).click()
  await page.getByLabel('Name', { exact: true }).fill('Colours')
  await page.getByLabel('Post in').selectOption('500')
  await page.getByRole('button', { name: 'Add a role' }).click()
  // Roles with dangerous permissions are never offered.
  await expect(page.getByLabel('Role 1', { exact: true }).locator('option', { hasText: '@Admins' })).toHaveCount(0)
  await page.getByLabel('Role 1', { exact: true }).selectOption('600')
  await page.getByLabel('Role 1 label').fill('Staff team')
  await expect(page.getByRole('figure', { name: 'Preview' })).toContainText('Staff team')
  await page.getByRole('button', { name: 'Save and post' }).click()
  await expect(page.getByText('Colours')).toBeVisible()
  expect(state.rolePanels[0]).toMatchObject({ name: 'Colours', channelId: '500', roles: [{ roleId: '600', label: 'Staff team' }] })
})

test('sets up a welcome message', async ({ page }) => {
  await page.goto('/guilds/100/welcome')
  await page.getByRole('switch', { name: 'Post a welcome message' }).click()
  await page.getByLabel('Channel').selectOption('500')
  await expect(page.getByRole('figure', { name: 'Preview' })).toContainText('Welcome to Test server, @ana!')
  await page.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByText('Saved')).toBeVisible()
  expect(state.welcome).toMatchObject({ welcomeEnabled: true, welcomeChannelId: '500', dmEnabled: false })
})

test('creates a ticket panel', async ({ page }) => {
  await page.goto('/guilds/100/tickets')
  await page.getByRole('button', { name: 'New panel' }).click()
  await page.getByLabel('Name', { exact: true }).fill('Help desk')
  await page.getByLabel('Post in').selectOption('500')
  await page.getByLabel('Button label').fill('Support')
  await page.getByRole('checkbox', { name: '@Staff' }).check()
  await page.getByRole('button', { name: 'Save and post' }).click()
  await expect(page.getByText('Help desk')).toBeVisible()
  expect(state.ticketPanels[0]).toMatchObject({ name: 'Help desk', categories: [{ label: 'Support', mode: 'thread', staffRoleIds: ['600'] }] })
})
