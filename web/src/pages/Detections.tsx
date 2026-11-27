import { Link } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { GuardianEvent, GuardianResolve, GuardianStatus } from '../api/client'
import { useGuardianEvents, useRaidStatus, useResolveEvent, useSetRaidMode } from '../api/queries'
import { Badge, Button, Card, Empty, ErrorState, Field, Lamp, Notice, RequestError, Select, Spinner, Status, useConfirm } from '../components/ui'
import { formatDateTime } from '../format'

const relFmt = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' })

function relative(iso: string) {
  const minutes = Math.round((new Date(iso).getTime() - Date.now()) / 60_000)
  return Math.abs(minutes) < 60 ? relFmt.format(minutes, 'minute') : relFmt.format(Math.round(minutes / 60), 'hour')
}

// A state has a mark: an open quarantine and an active raid wait (warning), a restored member is ok. A ban is
// an outcome told by its word, not a failure.
const statusTone: Record<GuardianStatus, 'neutral' | 'ok' | 'warning'> = {
  open: 'warning',
  kept: 'neutral',
  restored: 'ok',
  banned: 'neutral',
  info: 'neutral',
  active: 'warning',
  ended: 'neutral',
}

function RaidCard({ guildId }: { guildId: string }) {
  const { t } = useTranslation()
  const raid = useRaidStatus(guildId)
  const setRaid = useSetRaidMode(guildId)
  const [confirm, confirmDialog] = useConfirm()
  if (raid.isPending) return null
  if (raid.isError) return <RequestError error={raid.error} />
  const on = raid.data.active
  return (
    <Card className={on ? 'border-warning' : undefined}>
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <p className="flex items-center gap-2 font-[560]">
            {on && <Lamp state="warning" />}
            {on ? t('detections.raidOn') : t('detections.raidOff')}
          </p>
          <p className="mt-1 text-sm text-muted">
            {on && raid.data.endsAt ? t('detections.raidOnBody', { ends: relative(raid.data.endsAt) }) : t('detections.raidOffBody')}
          </p>
          {raid.data.problems.map((p) => (
            <p key={p} className="mt-1 text-sm">
              <Status state="warning">{t(`detections.problemText.${p}` as never, { defaultValue: p })}</Status>
            </p>
          ))}
        </div>
        <Button
          variant={on ? 'primary' : 'line'}
          disabled={setRaid.isPending}
          onClick={async () => {
            if (on || (await confirm({ message: t('detections.startRaidConfirm'), action: t('detections.startRaid') }))) setRaid.mutate(!on)
          }}
        >
          {on ? t('detections.endRaid') : t('detections.startRaid')}
        </Button>
      </div>
      <RequestError error={setRaid.error} />
      {confirmDialog}
    </Card>
  )
}

function EventCard({ guildId, ev }: { guildId: string; ev: GuardianEvent }) {
  const { t } = useTranslation()
  const resolve = useResolveEvent(guildId)
  const [confirm, confirmDialog] = useConfirm()
  const reviewable = ev.status === 'open' || ev.status === 'kept'
  const user = ev.userName ? `${ev.userName} (${ev.userId})` : ev.userId

  async function act(action: GuardianResolve) {
    const user = ev.userName || ev.userId
    if (action === 'ban' && !(await confirm({ message: t('detections.banConfirm', { user }), action: t('detections.banAction', { user }) }))) return
    resolve.mutate({ eventId: ev.id, action })
  }

  const rows: [string, React.ReactNode][] = []
  if (ev.userId) rows.push([t('detections.member'), user])
  if (ev.signals.length)
    rows.push([
      t('detections.why'),
      <ul key="why" className="grid gap-0.5">
        {ev.signals.map((s, i) => (
          <li key={i}>
            {t(`detections.signals.${s.kind}`)}
            {s.detail && s.kind !== 'manual' && <code className="ml-1 text-help text-muted">{s.detail}</code>}
          </li>
        ))}
      </ul>,
    ])
  if (ev.domains.length) rows.push([t('detections.domains'), <code key="d">{ev.domains.join(', ')}</code>])
  if (ev.channelIds.length && ev.kind !== 'raid') rows.push([t('detections.channels'), ev.channelIds.length])
  if (ev.deletedMessages) rows.push([t('detections.deleted'), ev.deletedMessages])
  if (ev.joins) rows.push([t('detections.joins'), ev.joins])
  if (ev.accountAge) rows.push([t('detections.accountAge'), ev.accountAge])
  if (!ev.monitor)
    rows.push([
      t('detections.action'),
      <span key="a">
        {t(`detections.responses.${ev.response}`)}
        {ev.caseNumber && (
          <>
            {' · '}
            <Link
              to="/guilds/$guildId/cases/$number"
              params={{ guildId, number: String(ev.caseNumber) }}
              className="underline underline-offset-2"
            >
              {t('detections.case', { number: ev.caseNumber })}
            </Link>
          </>
        )}
      </span>,
    ])

  return (
    <li className="py-5">
        <div className="flex flex-wrap items-center gap-3">
          <h3 className="font-[560]">{t(`detections.titles.${ev.kind}`)}</h3>
          <Badge tone={statusTone[ev.status]}>{t(`detections.statuses.${ev.status}`)}</Badge>
          <span className="text-help text-muted">
            #{ev.id} · <span className="font-mono">{formatDateTime(ev.createdAt)}</span>
            {ev.resolvedByName && ` · ${t('detections.resolvedBy', { name: ev.resolvedByName })}`}
          </span>
        </div>
        <dl className="mt-3 grid grid-cols-[max-content_1fr] gap-x-6 gap-y-1.5 text-sm">
          {rows.map(([k, v]) => (
            <div key={k} className="contents">
              <dt className="text-muted">{k}</dt>
              <dd>{v}</dd>
            </div>
          ))}
        </dl>
        {ev.monitor && <p className="mt-3 text-sm text-muted">{t('detections.monitor')}</p>}
        {ev.problems.length > 0 && (
          <Notice state="warning" className="mt-3">
            <p className="font-[560]">{t('detections.problems')}</p>
            <ul className="mt-1 list-disc pl-5">
              {ev.problems.map((p) => (
                <li key={p}>{t(`detections.problemText.${p}` as never, { defaultValue: p })}</li>
              ))}
            </ul>
          </Notice>
        )}
        {reviewable && (
          <div className="mt-4 flex flex-wrap gap-2">
            <Button size="compact" onClick={() => void act('restore')} disabled={resolve.isPending}>
              {t('detections.restore')}
            </Button>
            <Button variant="line" size="compact" onClick={() => void act('ban')} disabled={resolve.isPending}>
              {t('detections.ban')}
            </Button>
            {ev.status === 'open' && (
              <Button variant="ghost" size="compact" onClick={() => void act('keep')} disabled={resolve.isPending}>
                {t('detections.keep')}
              </Button>
            )}
          </div>
        )}
        <div className="mt-2">
          <RequestError error={resolve.error} />
          {confirmDialog}
        </div>
    </li>
  )
}

export function DetectionsPage({ guildId }: { guildId: string }) {
  const { t } = useTranslation()
  const [status, setStatus] = useState<'' | GuardianStatus>('')
  const events = useGuardianEvents(guildId, status || undefined)
  const items = events.data?.pages.flatMap((p) => p.items) ?? []

  return (
    <div>
      <h2 className="text-panel">{t('detections.title')}</h2>
      <p className="mt-1 text-muted">{t('detections.subtitle')}</p>

      <div className="mt-4">
        <RaidCard guildId={guildId} />
      </div>

      <div className="mt-6 w-56">
        <Field label={t('detections.filter')} htmlFor="det-status">
          <Select id="det-status" value={status} onChange={(e) => setStatus(e.target.value as '' | GuardianStatus)}>
            <option value="">{t('detections.filters.all')}</option>
            <option value="open">{t('detections.filters.open')}</option>
            <option value="kept">{t('detections.filters.kept')}</option>
          </Select>
        </Field>
      </div>

      <div className="mt-4">
        {events.isPending && <Spinner />}
        {events.isError && <ErrorState onRetry={() => void events.refetch()} />}
        {events.data && items.length === 0 && <Empty>{status ? t('detections.emptyFiltered') : t('detections.empty')}</Empty>}
        {items.length > 0 && (
          <ul className="divide-y divide-border border-y border-border">
            {items.map((ev) => (
              <EventCard key={ev.id} guildId={guildId} ev={ev} />
            ))}
          </ul>
        )}
        {events.hasNextPage && (
          <Button variant="line" className="mt-4" onClick={() => void events.fetchNextPage()} disabled={events.isFetchingNextPage}>
            {t('common.loadMore')}
          </Button>
        )}
      </div>
    </div>
  )
}
