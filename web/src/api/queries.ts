import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  api,
  ApiError,
  unwrap,
  type AutoModRule,
  type CaseAction,
  type ModerationSettings,
  type ModuleState,
} from './client'

export const keys = {
  me: ['me'] as const,
  guilds: ['guilds'] as const,
  guild: (id: string) => ['guilds', id] as const,
  modules: (id: string) => ['guilds', id, 'modules'] as const,
  invite: (id?: string) => ['invite', id ?? ''] as const,
}

/**
 * Don't retry errors the user has to act on (login, permissions), nor bot
 * errors: the API already waited for the bot, so retrying only makes the
 * user wait longer for the same answer.
 */
function retry(count: number, err: unknown) {
  if (err instanceof ApiError && err.status >= 400 && err.status < 500) return false
  if (err instanceof ApiError && (err.code === 'bot_unavailable' || err.code === 'discord_error')) return false
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

// ---- Moderation (M1) --------------------------------------------------------

export const modKeys = {
  meta: (g: string) => ['guilds', g, 'meta'] as const,
  settings: (g: string) => ['guilds', g, 'settings', 'moderation'] as const,
  cases: (g: string, filters: CaseFilters) => ['guilds', g, 'cases', filters] as const,
  allCases: (g: string) => ['guilds', g, 'cases'] as const,
  case: (g: string, n: number) => ['guilds', g, 'case', n] as const,
  automod: (g: string) => ['guilds', g, 'automod'] as const,
  presets: ['automod-presets'] as const,
}

export type CaseFilters = { user?: string; action?: CaseAction }

export function useGuildMeta(guildId: string) {
  return useQuery({
    queryKey: modKeys.meta(guildId),
    queryFn: () => unwrap(api.GET('/guilds/{guildId}/meta', { params: { path: { guildId } } })),
    staleTime: 30_000,
    retry,
  })
}

export function useModerationSettings(guildId: string) {
  return useQuery({
    queryKey: modKeys.settings(guildId),
    queryFn: () => unwrap(api.GET('/guilds/{guildId}/settings/moderation', { params: { path: { guildId } } })),
    retry,
  })
}

export function useSaveModerationSettings(guildId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: ModerationSettings) =>
      unwrap(api.PUT('/guilds/{guildId}/settings/moderation', { params: { path: { guildId } }, body })),
    onSuccess: (saved) => qc.setQueryData(modKeys.settings(guildId), saved),
  })
}

export function useCases(guildId: string, filters: CaseFilters) {
  return useInfiniteQuery({
    queryKey: modKeys.cases(guildId, filters),
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET('/guilds/{guildId}/cases', {
          params: { path: { guildId }, query: { ...filters, before: pageParam, limit: 25 } },
        }),
      ),
    initialPageParam: undefined as number | undefined,
    getNextPageParam: (last) => last.nextBefore,
    retry,
  })
}

export function useCase(guildId: string, number: number) {
  return useQuery({
    queryKey: modKeys.case(guildId, number),
    queryFn: () => unwrap(api.GET('/guilds/{guildId}/cases/{caseNumber}', { params: { path: { guildId, caseNumber: number } } })),
    retry,
  })
}

export function useUpdateCaseReason(guildId: string, number: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (reason: string) =>
      unwrap(api.PATCH('/guilds/{guildId}/cases/{caseNumber}', { params: { path: { guildId, caseNumber: number } }, body: { reason } })),
    onSuccess: (c) => {
      qc.setQueryData(modKeys.case(guildId, number), c)
      void qc.invalidateQueries({ queryKey: modKeys.allCases(guildId) })
    },
  })
}

export function useDeleteCase(guildId: string, number: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const { error, response } = await api.DELETE('/guilds/{guildId}/cases/{caseNumber}', {
        params: { path: { guildId, caseNumber: number } },
      })
      if (error !== undefined) throw new ApiError(response.status, error.code, error.message)
    },
    onSuccess: () => {
      qc.removeQueries({ queryKey: modKeys.case(guildId, number) })
      void qc.invalidateQueries({ queryKey: modKeys.allCases(guildId) })
    },
  })
}

export function useAutoModRules(guildId: string) {
  return useQuery({
    queryKey: modKeys.automod(guildId),
    queryFn: () => unwrap(api.GET('/guilds/{guildId}/automod/rules', { params: { path: { guildId } } })),
    retry,
  })
}

export function useAutoModPresets() {
  return useQuery({ queryKey: modKeys.presets, queryFn: () => unwrap(api.GET('/automod/presets')), staleTime: Infinity })
}

export function useSaveAutoModRule(guildId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (rule: AutoModRule) =>
      rule.id
        ? unwrap(api.PUT('/guilds/{guildId}/automod/rules/{ruleId}', { params: { path: { guildId, ruleId: rule.id } }, body: rule }))
        : unwrap(api.POST('/guilds/{guildId}/automod/rules', { params: { path: { guildId } }, body: rule })),
    onSuccess: () => void qc.invalidateQueries({ queryKey: modKeys.automod(guildId) }),
  })
}

export function useDeleteAutoModRule(guildId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (ruleId: string) => {
      const { error, response } = await api.DELETE('/guilds/{guildId}/automod/rules/{ruleId}', {
        params: { path: { guildId, ruleId } },
      })
      if (error !== undefined) throw new ApiError(response.status, error.code, error.message)
    },
    onSuccess: () => void qc.invalidateQueries({ queryKey: modKeys.automod(guildId) }),
  })
}
