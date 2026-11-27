import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { ModerationSettings } from '../api/client'
import { useGuildMeta, useModerationSettings, useModules, useSaveModerationSettings } from '../api/queries'
import { Button, ErrorState, Field, Notice, RequestError, Select, Spinner, Status, SwitchRow } from '../components/ui'

export function ModerationSettingsPage({ guildId }: { guildId: string }) {
  const { t } = useTranslation()
  const settings = useModerationSettings(guildId)
  const meta = useGuildMeta(guildId)
  const modules = useModules(guildId, true)
  const save = useSaveModerationSettings(guildId)
  // Local edits; null means "showing the saved settings".
  const [edits, setDraft] = useState<ModerationSettings | null>(null)

  if (settings.isError) return <ErrorState onRetry={() => void settings.refetch()} />
  if (settings.isPending) return <Spinner />
  const draft = edits ?? settings.data

  const moduleOff = modules.data?.find((m) => m.id === 'moderation')?.enabled === false
  const textChannels = (meta.data?.channels ?? []).filter((c) => c.type === 'text' || c.type === 'announcement')
  const dirty = JSON.stringify(draft) !== JSON.stringify(settings.data)

  return (
    <div className="max-w-2xl">
      <h2 className="text-panel">{t('modSettings.title')}</h2>
      <p className="mt-1 text-muted">{t('modSettings.subtitle')}</p>
      {moduleOff && <Notice className="mt-4">{t('modSettings.moduleOff')}</Notice>}

      <div className="mt-6 grid gap-6 border-t border-border pt-6">
        <Field label={t('modSettings.logChannel')} help={t('modSettings.logChannelHelp')} htmlFor="log-channel">
          <Select
            id="log-channel"
            value={draft.logChannelId}
            onChange={(e) => setDraft({ ...draft, logChannelId: e.target.value })}
            disabled={meta.isPending}
          >
            <option value="0">{t('modSettings.noLog')}</option>
            {textChannels.map((c) => (
              <option key={c.id} value={c.id}>
                #{c.name}
              </option>
            ))}
            {/* Keep the saved value selectable even if the channel list failed to load. */}
            {draft.logChannelId !== '0' && !textChannels.some((c) => c.id === draft.logChannelId) && (
              <option value={draft.logChannelId}>{draft.logChannelId}</option>
            )}
          </Select>
        </Field>
        {meta.isError && <RequestError error={meta.error} />}
        <SwitchRow
          label={t('modSettings.dmUsers')}
          help={t('modSettings.dmUsersHelp')}
          checked={draft.dmUsers}
          onCheckedChange={(v) => setDraft({ ...draft, dmUsers: v })}
        />
        <SwitchRow
          label={t('modSettings.requireReason')}
          help={t('modSettings.requireReasonHelp')}
          checked={draft.requireReason}
          onCheckedChange={(v) => setDraft({ ...draft, requireReason: v })}
        />
        <div className="flex items-center gap-4">
          <Button onClick={() => save.mutate(draft, { onSuccess: () => setDraft(null) })} disabled={!dirty || save.isPending}>
            {save.isPending ? t('common.saving') : t('common.save')}
          </Button>
          {save.isSuccess && !dirty && <Status state="ok" className="text-sm">{t('common.saved')}</Status>}
          <RequestError error={save.error} />
        </div>
      </div>
    </div>
  )
}
