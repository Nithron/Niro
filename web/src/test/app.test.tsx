import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, createMemoryHistory } from '@tanstack/react-router'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import '../i18n'
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
    const body = req.method === 'PUT' ? await req.clone().json() : undefined
    calls.push({ method: req.method, path: url.pathname, body })
    const handler = routes[`${req.method} ${url.pathname}`]
    return handler ? handler(req) : json({ code: 'not_found', message: 'nope' }, 404)
  })
})

afterEach(() => vi.unstubAllGlobals())

function renderAt(path: string) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = makeRouter(qc, createMemoryHistory({ initialEntries: [path] }))
  return render(
    <QueryClientProvider client={qc}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

describe('dashboard', () => {
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
