import { Link, useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError, type Case, type CaseAction } from '../api/client'
import { useCase, useCases, useDeleteCase, useUpdateCaseReason, type CaseFilters } from '../api/queries'
import { Icon } from '../components/Icon'
import { Badge, Button, Card, ErrorState, Field, RequestError, Select, Spinner, TextArea, TextInput, useConfirm } from '../components/ui'

export const caseActions: CaseAction[] = ['warn', 'timeout', 'untimeout', 'kick', 'ban', 'unban', 'note', 'purge', 'quarantine', 'unquarantine']

export function ActionBadge({ action }: { action: CaseAction }) {
  const { t } = useTranslation()
  // An action is told apart by its word, never by a colour: a ban is not a failure.
  return <Badge>{t(`cases.actions.${action}`)}</Badge>
}

const dateFmt = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' })

export function formatDuration(seconds: number): string {
  const units: [number, string][] = [
    [604800, 'w'],
    [86400, 'd'],
    [3600, 'h'],
    [60, 'm'],
  ]
  const parts: string[] = []
  for (const [size, label] of units) {
    const n = Math.floor(seconds / size)
    if (n > 0) {
      parts.push(`${n}${label}`)
      seconds -= n * size
    }
  }
  return parts.join(' ') || `${seconds}s`
}

export function CasesPage({ guildId, filters }: { guildId: string; filters: CaseFilters }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const cases = useCases(guildId, filters)
  const [userInput, setUserInput] = useState(filters.user ?? '')
  const items = cases.data?.pages.flatMap((p) => p.items) ?? []
  const total = cases.data?.pages[0]?.total ?? 0
  const filtered = Boolean(filters.user || filters.action)

  function setFilters(next: CaseFilters) {
    void navigate({ to: '/guilds/$guildId/cases', params: { guildId }, search: next })
  }

  return (
    <div>
      <h2 className="text-lg font-semibold">{t('cases.title')}</h2>
      <p className="mt-1 text-muted">{t('cases.subtitle')}</p>

      <form
        className="mt-4 flex flex-wrap items-end gap-3"
        onSubmit={(e) => {
          e.preventDefault()
          setFilters({ ...filters, user: /^\d{5,20}$/.test(userInput.trim()) ? userInput.trim() : undefined })
        }}
      >
        <div className="w-48">
          <Field label={t('cases.filterAction')} htmlFor="case-action">
            <Select
              id="case-action"
              value={filters.action ?? ''}
              onChange={(e) => setFilters({ ...filters, action: (e.target.value || undefined) as CaseAction | undefined })}
            >
              <option value="">{t('cases.allActions')}</option>
              {caseActions.map((a) => (
                <option key={a} value={a}>
                  {t(`cases.actions.${a}`)}
                </option>
              ))}
            </Select>
          </Field>
        </div>
        <div className="w-64">
          <Field label={t('cases.filterUser')} htmlFor="case-user">
            <TextInput
              id="case-user"
              inputMode="numeric"
              placeholder={t('cases.filterUserPlaceholder')}
              value={userInput}
              onChange={(e) => setUserInput(e.target.value)}
              onBlur={(e) => e.currentTarget.form?.requestSubmit()}
            />
          </Field>
        </div>
        {filtered && (
          <Button
            type="button"
            variant="ghost"
            onClick={() => {
              setUserInput('')
              setFilters({})
            }}
          >
            {t('common.clear')}
          </Button>
        )}
      </form>

      <div className="mt-4">
        {cases.isPending && <Spinner />}
        {cases.isError && <ErrorState onRetry={() => void cases.refetch()} />}
        {cases.data && items.length === 0 && (
          <Card>
            <p className="text-muted">{filtered ? t('cases.emptyFiltered') : t('cases.empty')}</p>
          </Card>
        )}
        {items.length > 0 && (
          <>
            <p className="mb-2 text-sm text-muted">{t('cases.total', { count: total })}</p>
            <div className="overflow-x-auto border border-border">
              <table className="w-full text-left text-sm">
                <thead className="bg-surface-2 text-xs text-muted uppercase">
                  <tr>
                    <th className="px-3 py-2">{t('cases.col.number')}</th>
                    <th className="px-3 py-2">{t('cases.col.action')}</th>
                    <th className="px-3 py-2">{t('cases.col.user')}</th>
                    <th className="px-3 py-2">{t('cases.col.moderator')}</th>
                    <th className="px-3 py-2">{t('cases.col.reason')}</th>
                    <th className="px-3 py-2">{t('cases.col.date')}</th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((c) => (
                    <CaseRow key={c.number} guildId={guildId} c={c} />
                  ))}
                </tbody>
              </table>
            </div>
            {cases.hasNextPage && (
              <Button variant="line" className="mt-4" onClick={() => void cases.fetchNextPage()} disabled={cases.isFetchingNextPage}>
                {t('common.loadMore')}
              </Button>
            )}
          </>
        )}
      </div>
    </div>
  )
}

function CaseRow({ guildId, c }: { guildId: string; c: Case }) {
  const { t } = useTranslation()
  return (
    <tr className="border-t border-border hover:bg-hover">
      <td className="px-3 py-2 font-mono">
        <Link to="/guilds/$guildId/cases/$number" params={{ guildId, number: String(c.number) }} className="underline underline-offset-2">
          #{c.number}
        </Link>
      </td>
      <td className="px-3 py-2">
        <ActionBadge action={c.action} />
      </td>
      <td className="px-3 py-2">{c.targetName || c.targetId || '—'}</td>
      <td className="px-3 py-2">{c.moderatorName}</td>
      <td className="max-w-xs truncate px-3 py-2 text-muted" title={c.reason}>
        {c.reason || t('cases.noReason')}
      </td>
      <td className="px-3 py-2 whitespace-nowrap text-muted">{dateFmt.format(new Date(c.createdAt))}</td>
    </tr>
  )
}

export function CaseDetailPage({ guildId, number }: { guildId: string; number: number }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const kase = useCase(guildId, number)
  const update = useUpdateCaseReason(guildId, number)
  const del = useDeleteCase(guildId, number)
  const [confirm, confirmDialog] = useConfirm()
  const [editing, setEditing] = useState(false)
  const [reason, setReason] = useState('')

  if (kase.isPending) return <Spinner />
  if (kase.isError) {
    const notFound = kase.error instanceof ApiError && kase.error.status === 404
    return <ErrorState message={notFound ? t('cases.notFound') : undefined} onRetry={notFound ? undefined : () => void kase.refetch()} />
  }
  const c = kase.data

  const rows: [string, React.ReactNode][] = [
    [t('cases.user'), c.targetId ? `${c.targetName || '—'} (${c.targetId})` : '—'],
    [t('cases.moderator'), `${c.moderatorName} (${c.moderatorId})`],
    [t('cases.created'), dateFmt.format(new Date(c.createdAt))],
    [t('cases.source'), t(`cases.sources.${c.source}`)],
  ]
  if (c.durationSeconds) rows.push([t('cases.duration'), formatDuration(c.durationSeconds) + (c.temporary ? ` · ${t('cases.temporary')}` : '')])
  if (c.expiresAt) rows.push([t('cases.expires'), dateFmt.format(new Date(c.expiresAt))])
  if (c.action === 'purge') {
    rows.push([t('cases.channel'), c.channelId ?? '—'])
    rows.push([t('cases.deleted'), String(c.deletedMessages ?? 0)])
  }
  if (['warn', 'timeout', 'kick', 'ban'].includes(c.action)) rows.push([t('cases.dm'), c.dmSent ? t('cases.dmSent') : t('cases.dmNotSent')])

  return (
    <div className="max-w-2xl">
      <Link to="/guilds/$guildId/cases" params={{ guildId }} search={{}} className="inline-flex items-center gap-1 text-sm text-muted hover:text-fg">
        <Icon name="back" />
        {t('cases.title')}
      </Link>
      <div className="mt-3 flex items-center gap-3">
        <h2 className="text-lg font-semibold">{t('cases.detail', { number: c.number })}</h2>
        <ActionBadge action={c.action} />
      </div>

      <Card className="mt-4">
        <dl className="grid grid-cols-[max-content_1fr] gap-x-6 gap-y-2 text-sm">
          {rows.map(([k, v]) => (
            <div key={k} className="contents">
              <dt className="text-muted">{k}</dt>
              <dd>{v}</dd>
            </div>
          ))}
        </dl>

        <div className="mt-6 border-t border-border pt-4">
          <p className="text-sm font-medium">{t('cases.reason')}</p>
          {!editing ? (
            <>
              <p className="mt-1 text-sm whitespace-pre-wrap">{c.reason || <span className="text-muted">{t('cases.noReason')}</span>}</p>
              <div className="mt-4 flex gap-2">
                <Button
                  variant="line"
                  onClick={() => {
                    setReason(c.reason)
                    setEditing(true)
                  }}
                >
                  {t('cases.editReason')}
                </Button>
                <Button
                  variant="line"
                  onClick={async () => {
                    if (await confirm({ message: t('cases.deleteConfirm', { number: c.number }), action: t('cases.deleteAction', { number: c.number }) })) {
                      del.mutate(undefined, { onSuccess: () => void navigate({ to: '/guilds/$guildId/cases', params: { guildId }, search: {} }) })
                    }
                  }}
                  disabled={del.isPending}
                >
                  {t('cases.deleteCase')}
                </Button>
              </div>
              <RequestError error={del.error} />
              {confirmDialog}
            </>
          ) : (
            <form
              className="mt-2 grid gap-3"
              onSubmit={(e) => {
                e.preventDefault()
                update.mutate(reason, { onSuccess: () => setEditing(false) })
              }}
            >
              <TextArea aria-label={t('cases.reason')} value={reason} maxLength={512} onChange={(e) => setReason(e.target.value)} className="font-sans" />
              <div className="flex gap-2">
                <Button type="submit" disabled={update.isPending}>
                  {update.isPending ? t('common.saving') : t('common.save')}
                </Button>
                <Button type="button" variant="ghost" onClick={() => setEditing(false)}>
                  {t('common.cancel')}
                </Button>
              </div>
              <RequestError error={update.error} />
            </form>
          )}
        </div>
      </Card>
    </div>
  )
}
