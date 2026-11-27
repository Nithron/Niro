import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type { LoggingSettings } from '../api/client'
import { useGuildMeta, useLoggingSettings, useModules, useSaveLoggingSettings } from '../api/queries'
import { Button, Card, ErrorState, Field, Notice, RequestError, Select, Spinner, Status, SwitchRow } from '../components/ui'

type Category = keyof LoggingSettings['channels']

/** Event kinds per category, in display order (matches internal/modules/logging). */
const categories = [
  { id: 'messages', kinds: ['message_edit', 'message_delete', 'message_bulk_delete'] },
  { id: 'members', kinds: ['member_join', 'member_leave', 'member_nick', 'member_roles'] },
  { id: 'moderation', kinds: ['member_ban', 'member_unban', 'member_kick', 'member_timeout'] },
  {
    id: 'server',
    kinds: ['channel_create', 'channel_update', 'channel_delete', 'role_create', 'role_update', 'role_delete', 'guild_update'],
  },
  { id: 'voice', kinds: ['voice_join', 'voice_leave', 'voice_move'] },
  { id: 'automod', kinds: ['automod_action'] },
  { id: 'guardian', kinds: ['guardian_action'] },
] as const satisfies readonly { id: Category; kinds: readonly string[] }[]

function Section({ title, help, children }: { title: string; help?: string; children: ReactNode }) {
  return (
    <Card className="grid gap-5">
      <div>
        <h3 className="font-semibold">{title}</h3>
        {help && <p className="mt-1 text-xs text-muted">{help}</p>}
      </div>
      {children}
    </Card>
  )
}

export function LoggingSettingsPage({ guildId }: { guildId: string }) {
  const { t } = useTranslation()
  const settings = useLoggingSettings(guildId)
  const meta = useGuildMeta(guildId)
  const modules = useModules(guildId, true)
  const save = useSaveLoggingSettings(guildId)
  // Local edits; null means "showing the saved settings".
  const [edits, setEdits] = useState<LoggingSettings | null>(null)

  if (settings.isError) return <ErrorState onRetry={() => void settings.refetch()} />
  if (settings.isPending) return <Spinner />
  const saved = settings.data
  const draft = edits ?? saved
  const set = <K extends keyof LoggingSettings>(k: K, v: LoggingSettings[K]) => setEdits({ ...draft, [k]: v })
  const dirty = JSON.stringify(draft) !== JSON.stringify(saved)

  const moduleOff = modules.data?.find((m) => m.id === 'logging')?.enabled === false
  const channels = meta.data?.channels ?? []
  const textChannels = channels.filter((c) => c.type === 'text' || c.type === 'announcement')
  const roles = (meta.data?.roles ?? []).filter((r) => r.id !== guildId).sort((a, b) => b.position - a.position)
  const nothingRouted = draft.defaultChannelId === '0' && Object.values(draft.channels).every((c) => c === '0')

  const channelSelect = (id: string, value: string, noneLabel: string, onChange: (v: string) => void) => (
    <Select id={id} value={value} onChange={(e) => onChange(e.target.value)} disabled={meta.isPending}>
      <option value="0">{noneLabel}</option>
      {textChannels.map((c) => (
        <option key={c.id} value={c.id}>
          #{c.name}
        </option>
      ))}
      {value !== '0' && !textChannels.some((c) => c.id === value) && <option value={value}>{value}</option>}
    </Select>
  )

  const toggle = (list: string[], id: string) => (list.includes(id) ? list.filter((x) => x !== id) : [...list, id])

  return (
    <div className="max-w-3xl">
      <h2 className="text-lg font-semibold">{t('loggingSettings.title')}</h2>
      <p className="mt-1 text-muted">{t('loggingSettings.subtitle')}</p>
      {moduleOff && <Notice className="mt-4">{t('loggingSettings.moduleOff')}</Notice>}
      {meta.isError && (
        <div className="mt-4">
          <RequestError error={meta.error} />
        </div>
      )}

      <div className="mt-6 grid gap-4">
        <Section title={t('loggingSettings.sections.channels')} help={t('loggingSettings.channelsHelp')}>
          <Field label={t('loggingSettings.defaultChannel')} help={t('loggingSettings.defaultChannelHelp')} htmlFor="l-default">
            {channelSelect('l-default', draft.defaultChannelId, t('loggingSettings.off'), (v) => set('defaultChannelId', v))}
          </Field>
          {nothingRouted && (
            <p className="text-sm">
              <Status state="warning">{t('loggingSettings.nothingRouted')}</Status>
            </p>
          )}
          <div className="grid gap-4 sm:grid-cols-2">
            {categories.map((c) => (
              <Field key={c.id} label={t(`loggingSettings.categories.${c.id}`)} htmlFor={`l-ch-${c.id}`}>
                {channelSelect(`l-ch-${c.id}`, draft.channels[c.id], t('loggingSettings.useDefault'), (v) =>
                  set('channels', { ...draft.channels, [c.id]: v }),
                )}
              </Field>
            ))}
          </div>
        </Section>

        <Section title={t('loggingSettings.sections.events')} help={t('loggingSettings.eventsHelp')}>
          {categories.map((c) => (
            <div key={c.id} className="grid gap-3">
              <h4 className="text-sm font-semibold text-muted">{t(`loggingSettings.categories.${c.id}`)}</h4>
              {c.kinds.map((k) => (
                <SwitchRow
                  key={k}
                  label={t(`loggingSettings.events.${k}`)}
                  checked={draft.events[k] ?? false}
                  onCheckedChange={(v) => set('events', { ...draft.events, [k]: v })}
                />
              ))}
            </div>
          ))}
        </Section>

        <Section title={t('loggingSettings.sections.privacy')}>
          <SwitchRow
            label={t('loggingSettings.noMessageContent')}
            help={t('loggingSettings.noMessageContentHelp')}
            checked={draft.noMessageContent}
            onCheckedChange={(v) => set('noMessageContent', v)}
          />
        </Section>

        <Section title={t('loggingSettings.sections.ignore')} help={t('loggingSettings.ignoreHelp')}>
          <SwitchRow
            label={t('loggingSettings.ignoreBots')}
            help={t('loggingSettings.ignoreBotsHelp')}
            checked={draft.ignoreBots}
            onCheckedChange={(v) => set('ignoreBots', v)}
          />
          <fieldset className="grid gap-2">
            <legend className="mb-2 text-sm font-medium">{t('loggingSettings.ignoreChannels')}</legend>
            <div className="grid gap-2 sm:grid-cols-2">
              {channels
                .filter((c) => c.type !== 'category')
                .map((c) => (
                  <label key={c.id} className="flex items-center gap-2 text-sm">
                    <input
                      type="checkbox"
                      checked={draft.ignoreChannelIds.includes(c.id)}
                      onChange={() => set('ignoreChannelIds', toggle(draft.ignoreChannelIds, c.id))}
                    />
                    #{c.name}
                  </label>
                ))}
            </div>
          </fieldset>
          <fieldset className="grid gap-2">
            <legend className="mb-2 text-sm font-medium">{t('loggingSettings.ignoreRoles')}</legend>
            <div className="grid gap-2 sm:grid-cols-2">
              {roles.map((r) => (
                <label key={r.id} className="flex items-center gap-2 text-sm">
                  <input
                    type="checkbox"
                    checked={draft.ignoreRoleIds.includes(r.id)}
                    onChange={() => set('ignoreRoleIds', toggle(draft.ignoreRoleIds, r.id))}
                  />
                  @{r.name}
                </label>
              ))}
            </div>
          </fieldset>
        </Section>

        <p className="text-xs text-muted">{t('loggingSettings.permissionsNote')}</p>
      </div>

      <div className="sticky bottom-0 mt-6 flex items-center gap-4 border-t border-border bg-bg py-4">
        <Button onClick={() => save.mutate(draft, { onSuccess: () => setEdits(null) })} disabled={!dirty || save.isPending}>
          {save.isPending ? t('common.saving') : t('common.save')}
        </Button>
        {save.isSuccess && !dirty && <Status state="ok" className="text-sm">{t('common.saved')}</Status>}
        <RequestError error={save.error} />
      </div>
    </div>
  )
}
