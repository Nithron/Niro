import { MutationCache, QueryCache, QueryClient, type DefaultOptions } from '@tanstack/react-query'
import { ApiError } from './api/client'
import { keys } from './api/queries'

/**
 * The app's QueryClient. Any 401 (logged out, or the session expired) makes
 * the app re-check who is logged in; the root route then shows the login
 * screen instead of a page-level error.
 */
export function makeQueryClient(defaults: DefaultOptions = {}): QueryClient {
  const is401 = (err: unknown) => err instanceof ApiError && err.status === 401
  const client: QueryClient = new QueryClient({
    queryCache: new QueryCache({
      onError: (err, query) => {
        // A 401 on the user query itself *is* the signed-out state; re-checking
        // it again would loop forever.
        if (is401(err) && query.queryKey[0] !== keys.me[0]) void signedOut(client)
      },
    }),
    mutationCache: new MutationCache({
      onError: (err) => {
        if (is401(err)) void signedOut(client)
      },
    }),
    defaultOptions: {
      ...defaults,
      queries: { staleTime: 15_000, refetchOnWindowFocus: true, ...defaults.queries },
    },
  })
  return client
}

/** Drops all cached data except the current user, then refetches the user. */
export async function signedOut(client: QueryClient): Promise<void> {
  client.removeQueries({ predicate: (q) => q.queryKey[0] !== keys.me[0] })
  await client.refetchQueries({ queryKey: keys.me })
}
