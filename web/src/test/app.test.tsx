import { QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, createMemoryHistory } from '@tanstack/react-router'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import '../i18n'
import { makeQueryClient } from '../queryClient'
import { makeRouter } from '../router'

type Handler = (req: Request) => Response | Promise<Response>

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

const me = { id: '7', username: 'ana', avatarUrl: 'https://cdn.discordapp.com/a.png' }
const guilds = [
  { id: '100', name: 'Beta', botPresent: true, owner: false },
  { id: '200', name: 'alpha', botPresent: false, owner: true },
]
const modules = [
  { id: 'core', enabled: true, alwaysOn: true, version: 0 },
  { id: 'logging', enabled: false, alwaysOn: false, version: 0 },
]

let routes: Record<string, Handler>
let calls: { method: string; path: string; body?: unknown }[]

beforeEach(() => {
  calls = []
  routes = {
    'GET /api/v1/me': () => json(me),
    'GET /api/v1/guilds': () => json(guilds),
    'GET /api/v1/guilds/100': () => json(guilds[0]),
    'GET /api/v1/guilds/100/modules': () => json(modules),
    'GET /api/v1/invite': () => json({ url: 'https://discord.com/oauth2/authorize?client_id=1' }),
  }
  vi.stubGlobal('fetch', async (input: RequestInfo | URL, init?: RequestInit) => {
    const req = input instanceof Request ? input : new Request(input, init)
    const url = new URL(req.url)
    const text = ['PUT', 'POST', 'PATCH'].includes(req.method) ? await req.clone().text() : ''
    const body = text ? JSON.parse(text) : undefined
    calls.push({ method: req.method, path: url.pathname, body })
    const handler = routes[`${req.method} ${url.pathname}`]
    return handler ? handler(req) : json({ code: 'not_found', message: 'nope' }, 404)
  })
})

afterEach(() => vi.unstubAllGlobals())

function renderAt(path: string) {
  const qc = makeQueryClient({ queries: { retry: false } })
  const router = makeRouter(qc, createMemoryHistory({ initialEntries: [path] }))
  return render(
    <QueryClientProvider client={qc}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

const unauthorized = () => json({ code: 'unauthorized', message: 'log in' }, 401)

describe('dashboard', () => {
  it('returns to the login screen after logging out', async () => {
    routes['POST /auth/logout'] = () => {
      routes['GET /api/v1/me'] = unauthorized
      routes['GET /api/v1/guilds'] = unauthorized
      return new Response(null, { status: 204 })
    }
    renderAt('/')
    await screen.findByText('Beta')
    await userEvent.click(screen.getByRole('button', { name: 'Log out' }))
    expect(await screen.findByRole('link', { name: 'Log in with Discord' })).toBeInTheDocument()
    expect(screen.queryByText('Beta')).not.toBeInTheDocument()
  })

  it('returns to the login screen when the session expires mid-use', async () => {
    renderAt('/')
    await screen.findByText('Beta')
    // The session expires; the next API call gets a 401.
    routes['GET /api/v1/me'] = unauthorized
    routes['GET /api/v1/guilds/100'] = unauthorized
    await userEvent.click(screen.getByRole('link', { name: 'Manage' }))
    expect(await screen.findByRole('link', { name: 'Log in with Discord' })).toBeInTheDocument()
    expect(screen.queryByText('Something went wrong. Please try again.')).not.toBeInTheDocument()
  })

  it('shows the Discord login when there is no session', async () => {
    routes['GET /api/v1/me'] = () => json({ code: 'unauthorized', message: 'log in' }, 401)
    renderAt('/')
    const button = await screen.findByRole('link', { name: 'Log in with Discord' })
    expect(button).toHaveAttribute('href', '/auth/login')
  })

  it('lists manageable servers with manage and invite actions', async () => {
    renderAt('/')
    expect(await screen.findByText('Beta')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Manage' })).toHaveAttribute('href', '/guilds/100')
    await waitFor(() =>
      expect(screen.getByRole('link', { name: 'Add Nithro' })).toHaveAttribute(
        'href',
        'https://discord.com/oauth2/authorize?client_id=1',
      ),
    )
  })

  it('toggles a module and saves it', async () => {
    routes['PUT /api/v1/guilds/100/modules/logging'] = () =>
      json({ id: 'logging', enabled: true, alwaysOn: false, version: 1, updatedAt: '2026-11-22T12:00:00Z' })
    renderAt('/guilds/100')

    const toggle = await screen.findByRole('switch', { name: 'Logging' })
    expect(toggle).toHaveAttribute('aria-checked', 'false')
    expect(screen.getByRole('switch', { name: 'Core' })).toBeDisabled()

    await userEvent.click(toggle)
    await waitFor(() => expect(toggle).toHaveAttribute('aria-checked', 'true'))
    expect(await screen.findByText('Saved')).toBeInTheDocument()
    expect(calls).toContainEqual({ method: 'PUT', path: '/api/v1/guilds/100/modules/logging', body: { enabled: true } })
  })

  it('rolls the toggle back when saving fails', async () => {
    routes['PUT /api/v1/guilds/100/modules/logging'] = () => json({ code: 'internal', message: 'boom' }, 500)
    renderAt('/guilds/100')

    const toggle = await screen.findByRole('switch', { name: 'Logging' })
    await userEvent.click(toggle)
    const alert = await screen.findByRole('alert')
    expect(within(alert).getByText(/undone/)).toBeInTheDocument()
    await waitFor(() => expect(toggle).toHaveAttribute('aria-checked', 'false'))
  })

  it('offers an invite when Nithro is not in the server', async () => {
    routes['GET /api/v1/guilds/200'] = () => json(guilds[1])
    renderAt('/guilds/200')
    expect(await screen.findByText("Nithro isn't in this server yet")).toBeInTheDocument()
    expect(calls.some((c) => c.path === '/api/v1/guilds/200/modules')).toBe(false)
  })
})

describe('moderation pages', () => {
  const meta = {
    channels: [
      { id: '500', name: 'mod-log', type: 'text', position: 1 },
      { id: '501', name: 'Lounge', type: 'voice', position: 2 },
    ],
    roles: [{ id: '600', name: 'Mods', color: 0, position: 3, managed: false }],
  }
  const kase = (n: number, extra: Record<string, unknown> = {}) => ({
    number: n, action: 'warn', targetId: '42', targetName: 'spammer', moderatorId: '7', moderatorName: 'ana',
    reason: `reason ${n}`, source: 'manual', dmSent: true, createdAt: '2026-11-22T12:00:00Z', updatedAt: '2026-11-22T12:00:00Z', ...extra,
  })

  beforeEach(() => {
    routes['GET /api/v1/guilds/100/meta'] = () => json(meta)
    routes['GET /api/v1/guilds/100/modules'] = () => json(modules)
  })

  it('saves moderation settings', async () => {
    routes['GET /api/v1/guilds/100/settings/moderation'] = () => json({ logChannelId: '0', dmUsers: true, requireReason: false })
    routes['PUT /api/v1/guilds/100/settings/moderation'] = async (req) => json(await req.json())
    renderAt('/guilds/100/moderation')

    const select = await screen.findByLabelText('Moderation log channel')
    await waitFor(() => expect(within(select).getByRole('option', { name: '#mod-log' })).toBeInTheDocument())
    expect(within(select).queryByRole('option', { name: '#Lounge' })).not.toBeInTheDocument() // voice channels can't hold logs
    await userEvent.selectOptions(select, '500')
    await userEvent.click(screen.getByRole('switch', { name: 'Require a reason' }))
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByText('Saved')).toBeInTheDocument()
    expect(calls).toContainEqual({
      method: 'PUT', path: '/api/v1/guilds/100/settings/moderation',
      body: { logChannelId: '500', dmUsers: true, requireReason: true },
    })
  })

  it('lists cases with filters and pagination', async () => {
    routes['GET /api/v1/guilds/100/cases'] = (req) => {
      const q = new URL(req.url).searchParams
      if (q.get('action') === 'ban') return json({ items: [kase(9, { action: 'ban' })], total: 1 })
      if (q.get('before') === '5') return json({ items: [4, 3, 2, 1].map((n) => kase(n)), total: 8 })
      return json({ items: [8, 7, 6, 5].map((n) => kase(n)), total: 8, nextBefore: 5 })
    }
    renderAt('/guilds/100/cases')

    expect(await screen.findByText('8 cases')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '#8' })).toHaveAttribute('href', '/guilds/100/cases/8')
    await userEvent.click(screen.getByRole('button', { name: 'Load more' }))
    expect(await screen.findByRole('link', { name: '#1' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Load more' })).not.toBeInTheDocument()

    await userEvent.selectOptions(screen.getByLabelText('Action'), 'ban')
    expect(await screen.findByText('1 case')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '#9' })).toBeInTheDocument()
  })

  it('edits a case reason', async () => {
    let reason = 'old'
    routes['GET /api/v1/guilds/100/cases/3'] = () => json(kase(3, { reason }))
    routes['PATCH /api/v1/guilds/100/cases/3'] = async (req) => {
      reason = ((await req.json()) as { reason: string }).reason
      return json(kase(3, { reason }))
    }
    renderAt('/guilds/100/cases/3')

    await userEvent.click(await screen.findByRole('button', { name: 'Edit reason' }))
    const box = screen.getByLabelText('Reason')
    await userEvent.clear(box)
    await userEvent.type(box, 'posted scam links')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByText('posted scam links')).toBeInTheDocument()
    expect(calls.some((c) => c.method === 'PATCH')).toBe(true)
  })

  it('adds an AutoMod preset and toggles rules', async () => {
    const preset = { id: 'invite_links', rule: { name: 'Nithro · Block invite links', enabled: true, trigger: 'keyword', blockMessage: true, regexPatterns: ['x'] } }
    let rules: Record<string, unknown>[] = [{ id: '1', name: 'Existing', enabled: true, trigger: 'spam', blockMessage: true }]
    routes['GET /api/v1/automod/presets'] = () => json([preset])
    routes['GET /api/v1/guilds/100/automod/rules'] = () => json(rules)
    routes['POST /api/v1/guilds/100/automod/rules'] = async (req) => {
      const r = { ...((await req.json()) as Record<string, unknown>), id: '2' }
      rules = [...rules, r]
      return json(r, 201)
    }
    routes['PUT /api/v1/guilds/100/automod/rules/1'] = async (req) => json(await req.json())
    renderAt('/guilds/100/automod')

    expect(await screen.findByText('Block invite links')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Add' }))
    expect(await screen.findByRole('button', { name: 'Added' })).toBeDisabled()
    expect(calls).toContainEqual(expect.objectContaining({ method: 'POST', path: '/api/v1/guilds/100/automod/rules' }))

    await userEvent.click(screen.getAllByRole('switch', { name: 'Enabled' })[0]!)
    await waitFor(() =>
      expect(calls).toContainEqual(expect.objectContaining({ method: 'PUT', path: '/api/v1/guilds/100/automod/rules/1', body: expect.objectContaining({ enabled: false }) })),
    )
  })

  it('explains when the bot is offline', async () => {
    routes['GET /api/v1/automod/presets'] = () => json([])
    routes['GET /api/v1/guilds/100/automod/rules'] = () => json({ code: 'bot_unavailable', message: 'x' }, 502)
    renderAt('/guilds/100/automod')
    expect(await screen.findByText(/The bot isn't responding/)).toBeInTheDocument()
  })
})
