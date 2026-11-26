import type { Page, Route } from '@playwright/test'

// A tiny in-memory stand-in for Niro's API, so the dashboard can be driven
// end to end without Discord, Postgres or Valkey.

export type Call = { method: string; path: string; body?: unknown }

const kinds = [
  'message_edit', 'message_delete', 'message_bulk_delete', 'member_join', 'member_leave', 'member_nick', 'member_roles',
  'member_ban', 'member_unban', 'member_kick', 'member_timeout', 'channel_create', 'channel_update', 'channel_delete',
  'role_create', 'role_update', 'role_delete', 'guild_update', 'voice_join', 'voice_leave', 'voice_move', 'automod_action',
  'guardian_action',
]

export function fakeState() {
  return {
    loggedIn: true,
    modules: [
      { id: 'core', enabled: true, alwaysOn: true, version: 0 },
      { id: 'moderation', enabled: true, alwaysOn: false, version: 0 },
      { id: 'guardian', enabled: true, alwaysOn: false, version: 0 },
      { id: 'logging', enabled: false, alwaysOn: false, version: 0 },
    ],
    guardian: {
      mode: 'enforce', alertChannelId: '0', exemptRoleIds: [] as string[], burstEnabled: true, burstChannels: 3,
      burstWindowSeconds: 60, quietDays: 30, scamLinksEnabled: true, allowDomains: [] as string[], denyDomains: [] as string[],
      quarantineMode: 'timeout', quarantineRoleId: '0', quarantineHours: 168, dmUser: true, raidEnabled: true, raidJoins: 10,
      raidWindowSeconds: 30, raidDurationMinutes: 30, raidPauseInvites: true, raidRaiseVerification: true, raidSlowmodeSeconds: 0,
      minAccountAgeHours: 0, accountAgeAction: 'kick', verifyRoleId: '0', verifyMinAccountAgeHours: 0,
    } as Record<string, unknown>,
    logging: {
      defaultChannelId: '0',
      channels: { messages: '0', members: '0', moderation: '0', server: '0', voice: '0', automod: '0', guardian: '0' },
      events: Object.fromEntries(kinds.map((k) => [k, !k.startsWith('voice_')])),
      ignoreChannelIds: [] as string[], ignoreRoleIds: [] as string[], ignoreBots: true, noMessageContent: false,
    } as Record<string, unknown>,
    welcome: {
      welcomeEnabled: false, welcomeChannelId: '0', pingMember: true, dmEnabled: false, goodbyeEnabled: false, goodbyeChannelId: '0',
      welcomeMessage: { version: 1, blocks: [{ type: 'text', text: 'Welcome to **{server}**, {user}!' }] },
      dmMessage: { version: 1, blocks: [{ type: 'text', text: 'Hi' }] },
      goodbyeMessage: { version: 1, blocks: [{ type: 'text', text: 'Bye {user.name}' }] },
      autoRoleIds: [] as string[],
    } as Record<string, unknown>,
    rolePanels: [] as Record<string, unknown>[],
    ticketPanels: [] as Record<string, unknown>[],
    calls: [] as Call[],
  }
}

export type FakeState = ReturnType<typeof fakeState>

const guild = { id: '100', name: 'Test server', botPresent: true, owner: true }
const meta = {
  channels: [
    { id: '500', name: 'logs', type: 'text', position: 0 },
    { id: '501', name: 'general', type: 'text', position: 1 },
  ],
  roles: [
    { id: '600', name: 'Staff', color: 0, position: 2, managed: false, dangerous: false },
    { id: '601', name: 'Admins', color: 0, position: 3, managed: false, dangerous: true },
  ],
}

export async function mockApi(page: Page, state: FakeState) {
  const reply = (route: Route, body: unknown, status = 200) =>
    route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })

  await page.route('**/api/v1/**', async (route) => {
    const req = route.request()
    const path = new URL(req.url()).pathname.replace(/^\/api\/v1/, '')
    const body = req.postData() ? JSON.parse(req.postData()!) : undefined
    state.calls.push({ method: req.method(), path, body })
    const key = `${req.method()} ${path}`

    if (!state.loggedIn) return reply(route, { code: 'unauthorized', message: 'log in' }, 401)
    switch (key) {
      case 'GET /me':
        return reply(route, { id: '7', username: 'ana', avatarUrl: 'https://cdn.discordapp.com/embed/avatars/0.png' })
      case 'GET /guilds':
        return reply(route, [guild])
      case 'GET /guilds/100':
        return reply(route, guild)
      case 'GET /guilds/100/modules':
        return reply(route, state.modules)
      case 'GET /guilds/100/meta':
        return reply(route, meta)
      case 'GET /invite':
        return reply(route, { url: 'https://discord.com/oauth2/authorize?client_id=1' })
      case 'GET /guilds/100/settings/guardian':
        return reply(route, state.guardian)
      case 'PUT /guilds/100/settings/guardian':
        state.guardian = body
        return reply(route, body)
      case 'GET /guilds/100/settings/logging':
        return reply(route, state.logging)
      case 'PUT /guilds/100/settings/logging':
        state.logging = body
        return reply(route, body)
      case 'GET /guilds/100/settings/welcome':
        return reply(route, state.welcome)
      case 'PUT /guilds/100/settings/welcome':
        state.welcome = body
        return reply(route, body)
      case 'GET /guilds/100/role-panels':
        return reply(route, { items: state.rolePanels })
      case 'POST /guilds/100/role-panels': {
        const p = { ...(body as object), id: state.rolePanels.length + 1, messageId: '900' }
        state.rolePanels.push(p)
        return reply(route, p, 201)
      }
      case 'GET /guilds/100/settings/tickets':
        return reply(route, { retentionDays: 90, transcriptChannelId: '0' })
      case 'GET /guilds/100/ticket-panels':
        return reply(route, { items: state.ticketPanels })
      case 'POST /guilds/100/ticket-panels': {
        const p = { ...(body as object), id: state.ticketPanels.length + 1, messageId: '901' }
        state.ticketPanels.push(p)
        return reply(route, p, 201)
      }
      case 'GET /guilds/100/tickets':
        return reply(route, { items: [] })
    }
    const mod = key.match(/^PUT \/guilds\/100\/modules\/(\w+)$/)
    if (mod) {
      const m = state.modules.find((x) => x.id === mod[1])
      if (!m) return reply(route, { code: 'not_found', message: 'no such module' }, 404)
      m.enabled = (body as { enabled: boolean }).enabled
      m.version++
      return reply(route, { ...m, updatedAt: new Date().toISOString() })
    }
    return reply(route, { code: 'not_found', message: key }, 404)
  })
}
