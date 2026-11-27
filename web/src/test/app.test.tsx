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
    await userEvent.click(screen.getByLabelText('Account'))
    await userEvent.click(screen.getByRole('button', { name: 'Log out' }))
    expect(await screen.findByRole('link', { name: 'Log in with Discord' })).toBeInTheDocument()
    expect(screen.queryByText('Beta')).not.toBeInTheDocument()
  })

  it('logs out on all devices', async () => {
    let called = false
    routes['POST /auth/logout-everywhere'] = () => {
      called = true
      routes['GET /api/v1/me'] = unauthorized
      routes['GET /api/v1/guilds'] = unauthorized
      return new Response(null, { status: 204 })
    }
    renderAt('/')
    await screen.findByText('Beta')
    await userEvent.click(screen.getByLabelText('Account'))
    await userEvent.click(screen.getByRole('button', { name: 'Log out on all devices' }))
    expect(await screen.findByRole('link', { name: 'Log in with Discord' })).toBeInTheDocument()
    expect(called).toBe(true)
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
      expect(screen.getByRole('link', { name: 'Add Niro' })).toHaveAttribute(
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

  it('offers an invite when Niro is not in the server', async () => {
    routes['GET /api/v1/guilds/200'] = () => json(guilds[1])
    renderAt('/guilds/200')
    expect(await screen.findByText("Niro isn't in this server yet")).toBeInTheDocument()
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
    const preset = { id: 'invite_links', rule: { name: 'Niro · Block invite links', enabled: true, trigger: 'keyword', blockMessage: true, regexPatterns: ['x'] } }
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

  it('lets the owner delete the server data after typing its name', async () => {
    let deleted = false
    routes['GET /api/v1/guilds/200'] = () => json(guilds[1])
    routes['DELETE /api/v1/guilds/200/data'] = () => {
      deleted = true
      return new Response(null, { status: 204 })
    }
    renderAt('/guilds/200')
    await userEvent.click(await screen.findByRole('button', { name: 'Delete data…' }))
    const confirm = screen.getByRole('button', { name: 'Remove Niro and delete everything' })
    expect(confirm).toBeDisabled()
    await userEvent.type(screen.getByLabelText('Type the server name (alpha) to confirm'), 'alph')
    expect(confirm).toBeDisabled()
    await userEvent.type(screen.getByLabelText('Type the server name (alpha) to confirm'), 'a')
    await userEvent.click(confirm)
    await waitFor(() => expect(deleted).toBe(true))
    expect(await screen.findByText('Your servers')).toBeInTheDocument()
  })

  it('hides data deletion from managers who are not the owner', async () => {
    renderAt('/guilds/100')
    await screen.findByRole('switch', { name: 'Logging' })
    expect(screen.queryByRole('button', { name: 'Delete data…' })).not.toBeInTheDocument()
  })

  it('shows the legal pages without logging in', async () => {
    routes['GET /api/v1/me'] = unauthorized
    renderAt('/privacy')
    expect(await screen.findByRole('heading', { level: 1, name: 'Privacy Policy' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Terms' })).toHaveAttribute('href', '/terms')
    expect(calls.some((c) => c.path === '/api/v1/me')).toBe(false)
  })

  it('explains a rate limit in plain words', async () => {
    routes['GET /api/v1/automod/presets'] = () => json([])
    routes['GET /api/v1/guilds/100/automod/rules'] = () => json({ code: 'rate_limited', message: 'Too many requests.' }, 429)
    renderAt('/guilds/100/automod')
    expect(await screen.findByText("You're going a bit fast. Wait a moment and try again.")).toBeInTheDocument()
  })

  it('explains when the bot is offline', async () => {
    routes['GET /api/v1/automod/presets'] = () => json([])
    routes['GET /api/v1/guilds/100/automod/rules'] = () => json({ code: 'bot_unavailable', message: 'x' }, 502)
    renderAt('/guilds/100/automod')
    expect(await screen.findByText(/The bot isn't responding/)).toBeInTheDocument()
  })
})

describe('guardian pages', () => {
  const settings = {
    mode: 'enforce', alertChannelId: '0', exemptRoleIds: [], burstEnabled: true, burstChannels: 3, burstWindowSeconds: 60,
    quietDays: 30, scamLinksEnabled: true, allowDomains: [], denyDomains: [], quarantineMode: 'timeout', quarantineRoleId: '0',
    quarantineHours: 168, dmUser: true, raidEnabled: true, raidJoins: 10, raidWindowSeconds: 30, raidDurationMinutes: 30,
    raidPauseInvites: true, raidRaiseVerification: true, raidSlowmodeSeconds: 0, minAccountAgeHours: 0, accountAgeAction: 'kick',
    verifyRoleId: '0', verifyMinAccountAgeHours: 0,
  }
  const event = (id: number, extra: Record<string, unknown> = {}) => ({
    id, userId: '42', userName: 'victim', kind: 'compromised', score: 3, response: 'quarantine', status: 'open', caseNumber: 4,
    signals: [{ kind: 'burst', detail: 'l', weight: 3 }], channelIds: ['11', '12', '13'], deletedMessages: 3,
    domains: ['evil.ru'], problems: [], monitor: false, createdAt: '2026-11-22T12:00:00Z', ...extra,
  })

  beforeEach(() => {
    routes['GET /api/v1/guilds/100/meta'] = () =>
      json({ channels: [{ id: '500', name: 'alerts', type: 'text', position: 0 }], roles: [{ id: '600', name: 'Quarantined', color: 0, position: 2, managed: false }] })
  })

  it('saves guardian settings and parses domain lists', async () => {
    routes['GET /api/v1/guilds/100/settings/guardian'] = () => json(settings)
    routes['PUT /api/v1/guilds/100/settings/guardian'] = async (req) => json(await req.json())
    renderAt('/guilds/100/guardian')

    await userEvent.selectOptions(await screen.findByLabelText('Mode'), 'monitor')
    await userEvent.type(screen.getByLabelText('Always block these domains', { selector: '#g-deny' }), 'grabify.link{enter}bad.example')
    await userEvent.selectOptions(screen.getByLabelText('How to restrict'), 'role')
    await userEvent.selectOptions(await screen.findByLabelText('Quarantine role'), '600')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() =>
      expect(calls).toContainEqual(
        expect.objectContaining({
          method: 'PUT',
          body: expect.objectContaining({ mode: 'monitor', denyDomains: ['grabify.link', 'bad.example'], quarantineMode: 'role', quarantineRoleId: '600' }),
        }),
      ),
    )
    expect(await screen.findByText('Saved')).toBeInTheDocument()
  })

  it('reviews a detection and controls raid mode', async () => {
    let events = [event(2), event(1, { kind: 'scam', response: 'delete', status: 'info', caseNumber: undefined })]
    let raid: Record<string, unknown> = { active: false, problems: [] }
    routes['GET /api/v1/guilds/100/guardian/events'] = () => json({ items: events })
    routes['POST /api/v1/guilds/100/guardian/events/2/resolve'] = () => {
      events = [event(2, { status: 'restored', resolvedByName: 'ana' }), events[1]!]
      return json(events[0])
    }
    routes['GET /api/v1/guilds/100/guardian/raid'] = () => json(raid)
    routes['POST /api/v1/guilds/100/guardian/raid'] = () => {
      raid = { active: true, problems: [], endsAt: new Date(Date.now() + 30 * 60_000).toISOString() }
      return json(raid)
    }
    vi.stubGlobal('confirm', () => true)
    renderAt('/guilds/100/guardian/detections')

    expect(await screen.findByText('Possible hacked account')).toBeInTheDocument()
    expect(screen.getAllByText('Same message in several channels')).toHaveLength(2)
    expect(screen.getByRole('link', { name: 'Case #4' })).toHaveAttribute('href', '/guilds/100/cases/4')
    // Only the open quarantine can be reviewed.
    expect(screen.getAllByRole('button', { name: 'Restore' })).toHaveLength(1)

    await userEvent.click(screen.getByRole('button', { name: 'Restore' }))
    expect(await screen.findByText('Restored')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Restore' })).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Start raid mode' }))
    expect(await screen.findByText('Raid mode is on')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'End raid mode' })).toBeInTheDocument()
  })

  it('shows the permission audit', async () => {
    routes['GET /api/v1/guilds/100/guardian/audit'] = () =>
      json({
        webhooksChecked: false,
        findings: [
          { severity: 'critical', code: 'everyone_dangerous', permissions: ['administrator'] },
          { severity: 'high', code: 'role_admin', subject: 'Admins', count: -1 },
        ],
      })
    renderAt('/guilds/100/guardian/audit')
    expect(await screen.findByText('@everyone has Administrator: every member can use these.')).toBeInTheDocument()
    expect(screen.getByText('Role Admins has Administrator (? members).')).toBeInTheDocument()
    expect(screen.getByText(/Manage Webhooks permission/)).toBeInTheDocument()
  })
})

describe('logging page', () => {
  const kinds = [
    'message_edit', 'message_delete', 'message_bulk_delete', 'member_join', 'member_leave', 'member_nick', 'member_roles',
    'member_ban', 'member_unban', 'member_kick', 'member_timeout', 'channel_create', 'channel_update', 'channel_delete',
    'role_create', 'role_update', 'role_delete', 'guild_update', 'voice_join', 'voice_leave', 'voice_move', 'automod_action',
    'guardian_action',
  ]
  const settings = {
    defaultChannelId: '0',
    channels: { messages: '0', members: '0', moderation: '0', server: '0', voice: '0', automod: '0', guardian: '0' },
    events: Object.fromEntries(kinds.map((k) => [k, !k.startsWith('voice_')])),
    ignoreChannelIds: [], ignoreRoleIds: [], ignoreBots: true, noMessageContent: false,
  }

  beforeEach(() => {
    routes['GET /api/v1/guilds/100/meta'] = () =>
      json({
        channels: [
          { id: '500', name: 'logs', type: 'text', position: 0 },
          { id: '501', name: 'staff', type: 'text', position: 1 },
        ],
        roles: [{ id: '600', name: 'Staff', color: 0, position: 2, managed: false }],
      })
  })

  it('routes categories, toggles events and saves', async () => {
    routes['GET /api/v1/guilds/100/settings/logging'] = () => json(settings)
    routes['PUT /api/v1/guilds/100/settings/logging'] = async (req) => json(await req.json())
    renderAt('/guilds/100/logging')

    expect(await screen.findByText('No log channel is set, so nothing is logged yet.')).toBeInTheDocument()
    await userEvent.selectOptions(await screen.findByLabelText('Default log channel'), '500')
    await userEvent.selectOptions(screen.getByLabelText('Moderation'), '501')
    await userEvent.click(screen.getByRole('switch', { name: 'Voice joins' }))
    await userEvent.click(screen.getByRole('switch', { name: "Don't log message text" }))
    await userEvent.click(screen.getByRole('checkbox', { name: '@Staff' }))
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() =>
      expect(calls).toContainEqual(
        expect.objectContaining({
          method: 'PUT',
          body: expect.objectContaining({
            defaultChannelId: '500',
            channels: expect.objectContaining({ moderation: '501', messages: '0' }),
            events: expect.objectContaining({ voice_join: true, message_delete: true }),
            noMessageContent: true,
            ignoreRoleIds: ['600'],
          }),
        }),
      ),
    )
    expect(await screen.findByText('Saved')).toBeInTheDocument()
  })
})

describe('role panels page', () => {
  beforeEach(() => {
    routes['GET /api/v1/guilds/100/meta'] = () =>
      json({
        channels: [{ id: '500', name: 'roles', type: 'text', position: 0 }],
        roles: [
          { id: '600', name: 'Red', color: 0, position: 3, managed: false, dangerous: false },
          { id: '601', name: 'Admins', color: 0, position: 4, managed: false, dangerous: true },
          { id: '602', name: 'Bot', color: 0, position: 5, managed: true, dangerous: false },
        ],
      })
    routes['GET /api/v1/guilds/100/role-panels'] = () => json({ items: [] })
  })

  it('builds a panel, previews it and saves it', async () => {
    routes['POST /api/v1/guilds/100/role-panels'] = async (req) => json({ ...(await req.json()), id: 1, messageId: '900' }, 201)
    renderAt('/guilds/100/roles')
    await userEvent.click(await screen.findByRole('button', { name: 'New panel' }))

    await userEvent.type(screen.getByLabelText('Name'), 'Colours')
    await userEvent.selectOptions(screen.getByLabelText('Post in'), '500')
    await userEvent.selectOptions(screen.getByLabelText('Mode'), 'unique')
    await userEvent.click(screen.getByRole('button', { name: 'Add a role' }))
    const role = screen.getByLabelText('Role 1')
    expect(within(role).queryByText('@Admins')).toBeNull()
    expect(within(role).queryByText('@Bot')).toBeNull()
    expect(screen.getByText(/dangerous permissions: @Admins, @Bot|dangerous permissions: @Bot, @Admins/)).toBeInTheDocument()
    await userEvent.selectOptions(role, '600')
    await userEvent.type(screen.getByLabelText('Role 1 label'), 'Red team')
    expect(screen.getByRole('figure', { name: 'Preview' })).toHaveTextContent('Red team')

    await userEvent.click(screen.getByRole('button', { name: 'Save and post' }))
    await waitFor(() =>
      expect(calls).toContainEqual(
        expect.objectContaining({
          method: 'POST',
          path: '/api/v1/guilds/100/role-panels',
          body: expect.objectContaining({
            name: 'Colours', channelId: '500', mode: 'unique', style: 'buttons',
            roles: [{ roleId: '600', label: 'Red team' }],
          }),
        }),
      ),
    )
    expect(await screen.findByRole('button', { name: 'New panel' })).toBeInTheDocument()
  })
})

describe('welcome page', () => {
  const doc = (text: string) => ({ version: 1, blocks: [{ type: 'text', text }] })
  const settings = {
    welcomeEnabled: false, welcomeChannelId: '0', welcomeMessage: doc('Welcome to **{server}**, {user}!'), pingMember: true,
    dmEnabled: false, dmMessage: doc('Hi'), goodbyeEnabled: false, goodbyeChannelId: '0', goodbyeMessage: doc('Bye {user.name}'),
    autoRoleIds: [],
  }
  beforeEach(() => {
    routes['GET /api/v1/guilds/100/meta'] = () =>
      json({
        channels: [{ id: '500', name: 'welcome', type: 'text', position: 0 }],
        roles: [
          { id: '600', name: 'Member', color: 0, position: 1, managed: false, dangerous: false },
          { id: '601', name: 'Admins', color: 0, position: 2, managed: false, dangerous: true },
        ],
      })
  })

  it('previews variables and saves', async () => {
    routes['GET /api/v1/guilds/100/settings/welcome'] = () => json(settings)
    routes['PUT /api/v1/guilds/100/settings/welcome'] = async (req) => json(await req.json())
    renderAt('/guilds/100/welcome')
    await userEvent.click(await screen.findByRole('switch', { name: 'Post a welcome message' }))
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled() // no channel yet
    await userEvent.selectOptions(screen.getByLabelText('Channel'), '500')
    expect(screen.getByRole('figure', { name: 'Preview' })).toHaveTextContent('Welcome to Beta, @ana!')
    expect(screen.getByRole('checkbox', { name: /@Admins/ })).toBeDisabled()
    await userEvent.click(screen.getByRole('checkbox', { name: '@Member' }))
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(calls).toContainEqual(
        expect.objectContaining({
          method: 'PUT',
          body: expect.objectContaining({ welcomeEnabled: true, welcomeChannelId: '500', autoRoleIds: ['600'], dmEnabled: false }),
        }),
      ),
    )
  })
})

describe('tickets page', () => {
  beforeEach(() => {
    routes['GET /api/v1/guilds/100/meta'] = () =>
      json({
        channels: [
          { id: '500', name: 'help', type: 'text', position: 0 },
          { id: '510', name: 'Tickets', type: 'category', position: 1 },
        ],
        roles: [{ id: '600', name: 'Staff', color: 0, position: 1, managed: false, dangerous: false }],
      })
    routes['GET /api/v1/guilds/100/settings/tickets'] = () => json({ retentionDays: 90, transcriptChannelId: '0' })
    routes['GET /api/v1/guilds/100/ticket-panels'] = () => json({ items: [] })
    routes['GET /api/v1/guilds/100/tickets'] = () =>
      json({
        items: [
          { number: 2, categoryLabel: 'Support', mode: 'thread', openerId: '7', openerName: 'ana', status: 'closed', createdAt: '2026-11-26T10:00:00Z', hasTranscript: true },
          { number: 1, categoryLabel: 'Support', mode: 'thread', openerId: '8', openerName: 'bo', status: 'open', createdAt: '2026-11-26T09:00:00Z', hasTranscript: false },
        ],
      })
  })

  it('lists tickets with transcript links', async () => {
    renderAt('/guilds/100/tickets')
    const link = await screen.findByRole('link', { name: 'Transcript' })
    expect(link).toHaveAttribute('href', '/api/v1/guilds/100/tickets/2/transcript')
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')
    expect(screen.getAllByRole('link', { name: 'Transcript' })).toHaveLength(1)
  })

  it('creates a panel with a category, staff and a question', async () => {
    routes['POST /api/v1/guilds/100/ticket-panels'] = async (req) => json({ ...(await req.json()), id: 1, messageId: '900' }, 201)
    renderAt('/guilds/100/tickets')
    await userEvent.click(await screen.findByRole('button', { name: 'New panel' }))
    await userEvent.type(screen.getByLabelText('Name'), 'Help desk')
    await userEvent.selectOptions(screen.getByLabelText('Post in'), '500')
    await userEvent.type(screen.getByLabelText('Button label'), 'Support')
    await userEvent.selectOptions(screen.getByLabelText('Ticket type'), 'channel')
    await userEvent.selectOptions(screen.getByLabelText('Create channels in'), '510')
    await userEvent.click(screen.getByRole('checkbox', { name: '@Staff' }))
    await userEvent.click(screen.getByRole('button', { name: 'Add a question' }))
    await userEvent.type(screen.getByLabelText('Question 1'), "What's wrong?")
    expect(screen.getByRole('figure', { name: 'Preview' })).toHaveTextContent('Support')
    await userEvent.click(screen.getByRole('button', { name: 'Save and post' }))
    await waitFor(() =>
      expect(calls).toContainEqual(
        expect.objectContaining({
          method: 'POST',
          path: '/api/v1/guilds/100/ticket-panels',
          body: expect.objectContaining({
            name: 'Help desk', channelId: '500',
            categories: [expect.objectContaining({ label: 'Support', mode: 'channel', parentId: '510', staffRoleIds: ['600'],
              questions: [{ label: "What's wrong?", long: false, required: true }] })],
          }),
        }),
      ),
    )
  })
})
