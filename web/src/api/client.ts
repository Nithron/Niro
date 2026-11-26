import createClient from 'openapi-fetch'
import type { components, paths } from './schema'

export type User = components['schemas']['User']
export type GuildSummary = components['schemas']['GuildSummary']
export type ModuleState = components['schemas']['ModuleState']
export type ErrorCode = components['schemas']['Error']['code']

export class ApiError extends Error {
  readonly status: number
  readonly code: ErrorCode

  constructor(status: number, code: ErrorCode, message: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

// fetch is looked up on every call (not captured once) so tests can stub it.
export const api = createClient<paths>({
  baseUrl: new URL('/api/v1', window.location.origin).toString(),
  credentials: 'same-origin',
  fetch: (req) => globalThis.fetch(req),
})

type Result<T> = { data?: T; error?: unknown; response: Response }

/** Returns the data of an openapi-fetch result or throws an ApiError. */
export async function unwrap<T>(p: Promise<Result<T>>): Promise<T> {
  const { data, error, response } = await p
  if (error !== undefined || data === undefined) {
    const e = (error ?? {}) as Partial<components['schemas']['Error']>
    throw new ApiError(response.status, e.code ?? 'internal', e.message ?? response.statusText)
  }
  return data
}

export async function logout(): Promise<void> {
  await globalThis.fetch(new URL('/auth/logout', window.location.origin), {
    method: 'POST',
    credentials: 'same-origin',
  })
}

export type GuildMeta = components['schemas']['GuildMeta']
export type ModerationSettings = components['schemas']['ModerationSettings']
export type Case = components['schemas']['Case']
export type CaseAction = components['schemas']['CaseAction']
export type CaseList = components['schemas']['CaseList']
export type AutoModRule = components['schemas']['AutoModRule']
export type AutoModPreset = components['schemas']['AutoModPreset']
export type GuardianSettings = components['schemas']['GuardianSettings']
export type LoggingSettings = components['schemas']['LoggingSettings']
export type RolePanel = components['schemas']['RolePanel']
export type RolePanelRole = components['schemas']['RolePanelRole']
export type GuardianEvent = components['schemas']['GuardianEvent']
export type GuardianStatus = components['schemas']['GuardianStatus']
export type GuardianResolve = components['schemas']['GuardianResolve']['action']
export type RaidStatus = components['schemas']['RaidStatus']
export type AuditResult = components['schemas']['AuditResult']
export type AuditFinding = components['schemas']['AuditFinding']
