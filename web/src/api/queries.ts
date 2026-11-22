import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, unwrap, type ModuleState } from './client'

export const keys = {
  me: ['me'] as const,
  guilds: ['guilds'] as const,
  guild: (id: string) => ['guilds', id] as const,
  modules: (id: string) => ['guilds', id, 'modules'] as const,
  invite: (id?: string) => ['invite', id ?? ''] as const,
}

/** Don't retry errors the user has to act on (login, permissions). */
function retry(count: number, err: unknown) {
  if (err instanceof ApiError && err.status >= 400 && err.status < 500) return false
  return count < 2
}

export function useMe() {
  return useQuery({ queryKey: keys.me, queryFn: () => unwrap(api.GET('/me')), retry })
}

export function useGuilds() {
  return useQuery({ queryKey: keys.guilds, queryFn: () => unwrap(api.GET('/guilds')), retry })
}

export function useGuild(guildId: string) {
  return useQuery({
    queryKey: keys.guild(guildId),
    queryFn: () => unwrap(api.GET('/guilds/{guildId}', { params: { path: { guildId } } })),
    retry,
  })
}

export function useModules(guildId: string, enabled: boolean) {
  return useQuery({
    queryKey: keys.modules(guildId),
    queryFn: () => unwrap(api.GET('/guilds/{guildId}/modules', { params: { path: { guildId } } })),
    enabled,
    retry,
  })
}

export function useInvite(guildId?: string) {
  return useQuery({
    queryKey: keys.invite(guildId),
    queryFn: () => unwrap(api.GET('/invite', { params: { query: guildId ? { guildId } : {} } })),
    staleTime: Infinity,
  })
}

/** Toggles a module with an optimistic update that is rolled back on failure. */
export function useSetModule(guildId: string) {
  const qc = useQueryClient()
  const key = keys.modules(guildId)
  return useMutation({
    mutationFn: ({ moduleId, enabled }: { moduleId: string; enabled: boolean }) =>
      unwrap(
        api.PUT('/guilds/{guildId}/modules/{moduleId}', {
          params: { path: { guildId, moduleId } },
          body: { enabled },
        }),
      ),
    onMutate: async ({ moduleId, enabled }) => {
      await qc.cancelQueries({ queryKey: key })
      const previous = qc.getQueryData<ModuleState[]>(key)
      qc.setQueryData<ModuleState[]>(key, (mods) =>
        mods?.map((m) => (m.id === moduleId ? { ...m, enabled } : m)),
      )
      return { previous }
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.previous) qc.setQueryData(key, ctx.previous)
    },
    onSuccess: (updated) => {
      qc.setQueryData<ModuleState[]>(key, (mods) => mods?.map((m) => (m.id === updated.id ? updated : m)))
    },
  })
}
