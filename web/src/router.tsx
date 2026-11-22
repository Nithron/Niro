import type { QueryClient } from '@tanstack/react-query'
import {
  Outlet,
  createRootRouteWithContext,
  createRoute,
  createRouter,
  type RouterHistory,
} from '@tanstack/react-router'
import { ApiError, type CaseAction } from './api/client'
import { useMe, type CaseFilters } from './api/queries'
import { Layout } from './components/Layout'
import { ErrorState, Spinner } from './components/ui'
import { AutoModPage } from './pages/AutoMod'
import { CaseDetailPage, CasesPage, caseActions } from './pages/Cases'
import { GuildLayout, ModulesPage } from './pages/Guild'
import { LoginPage } from './pages/Login'
import { ModerationSettingsPage } from './pages/ModerationSettings'
import { ServersPage } from './pages/Servers'

/** Every page needs a logged-in user; without one, show the login screen. */
function Root() {
  const me = useMe()
  if (me.isPending) return <Spinner />
  if (me.isError) {
    if (me.error instanceof ApiError && me.error.status === 401) return <LoginPage />
    return (
      <div className="mx-auto max-w-md px-4 py-16">
        <ErrorState onRetry={() => void me.refetch()} />
      </div>
    )
  }
  return (
    <Layout user={me.data}>
      <Outlet />
    </Layout>
  )
}

const rootRoute = createRootRouteWithContext<{ queryClient: QueryClient }>()({ component: Root })

const indexRoute = createRoute({ getParentRoute: () => rootRoute, path: '/', component: ServersPage })

const guildRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/guilds/$guildId',
  component: function GuildRoute() {
    const { guildId } = guildRoute.useParams()
    return <GuildLayout guildId={guildId} />
  },
})

const modulesRoute = createRoute({
  getParentRoute: () => guildRoute,
  path: '/',
  component: function ModulesRoute() {
    const { guildId } = guildRoute.useParams()
    return <ModulesPage guildId={guildId} />
  },
})

const moderationRoute = createRoute({
  getParentRoute: () => guildRoute,
  path: '/moderation',
  component: function ModerationRoute() {
    const { guildId } = guildRoute.useParams()
    return <ModerationSettingsPage guildId={guildId} />
  },
})

const casesRoute = createRoute({
  getParentRoute: () => guildRoute,
  path: '/cases',
  // Filters live in the URL so the bot's /history link can deep-link here.
  validateSearch: (search: Record<string, unknown>): CaseFilters => ({
    user: typeof search.user === 'string' && /^\d{1,20}$/.test(search.user) ? search.user : undefined,
    action: caseActions.includes(search.action as CaseAction) ? (search.action as CaseAction) : undefined,
  }),
  component: function CasesRoute() {
    const { guildId } = guildRoute.useParams()
    const filters = casesRoute.useSearch()
    return <CasesPage guildId={guildId} filters={filters} />
  },
})

const caseRoute = createRoute({
  getParentRoute: () => guildRoute,
  path: '/cases/$number',
  component: function CaseRoute() {
    const { guildId } = guildRoute.useParams()
    const { number } = caseRoute.useParams()
    return <CaseDetailPage guildId={guildId} number={Number(number)} />
  },
})

const automodRoute = createRoute({
  getParentRoute: () => guildRoute,
  path: '/automod',
  component: function AutoModRoute() {
    const { guildId } = guildRoute.useParams()
    return <AutoModPage guildId={guildId} />
  },
})

const routeTree = rootRoute.addChildren([
  indexRoute,
  guildRoute.addChildren([modulesRoute, moderationRoute, casesRoute, caseRoute, automodRoute]),
])

export function makeRouter(queryClient: QueryClient, history?: RouterHistory) {
  return createRouter({ routeTree, context: { queryClient }, defaultPreload: 'intent', history })
}

declare module '@tanstack/react-router' {
  interface Register {
    router: ReturnType<typeof makeRouter>
  }
}
