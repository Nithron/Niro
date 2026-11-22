import type { QueryClient } from '@tanstack/react-query'
import {
  Outlet,
  createRootRouteWithContext,
  createRoute,
  createRouter,
  type RouterHistory,
} from '@tanstack/react-router'
import { ApiError } from './api/client'
import { useMe } from './api/queries'
import { Layout } from './components/Layout'
import { ErrorState, Spinner } from './components/ui'
import { GuildPage } from './pages/Guild'
import { LoginPage } from './pages/Login'
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
    return <GuildPage guildId={guildId} />
  },
})

const routeTree = rootRoute.addChildren([indexRoute, guildRoute])

export function makeRouter(queryClient: QueryClient, history?: RouterHistory) {
  return createRouter({ routeTree, context: { queryClient }, defaultPreload: 'intent', history })
}

declare module '@tanstack/react-router' {
  interface Register {
    router: ReturnType<typeof makeRouter>
  }
}
