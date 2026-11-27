import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { WelcomeSettings } from '../api/client'
import { useGuild, useGuildMeta, useMe, useModules, useSaveWelcomeSettings, useWelcomeSettings } from '../api/queries'
import { MessageBuilder } from '../components/builder/MessageBuilder'
import { MessagePreview } from '../components/builder/MessagePreview'
import { renderVariables, type PreviewVars } from '../components/builder/vars'
import { Button, ErrorState, Field, Notice, Panel, RequestError, Select, Spinner, Status, SwitchRow } from '../components/ui'

const MAX_AUTO_ROLES = 10


export function WelcomeSettingsPage({ guildId }: { guildId: string }) {
  const { t } = useTranslation()
  const settings = useWelcomeSettings(guildId)
  const meta = useGuildMeta(guildId)
  const modules = useModules(guildId, true)
  const me = useMe()
  const guild = useGuild(guildId)
  const save = useSaveWelcomeSettings(guildId)
  const [edits, setEdits] = useState<WelcomeSettings | null>(null)

  if (settings.isError) return <ErrorState onRetry={() => void settings.refetch()} />
  if (settings.isPending) return <Spinner />
  const saved = settings.data
  const draft = edits ?? saved
  const set = <K extends keyof WelcomeSettings>(k: K, v: WelcomeSettings[K]) => setEdits((prev) => ({ ...(prev ?? saved), [k]: v }))
  const dirty = JSON.stringify(draft) !== JSON.stringify(saved)

  const moduleOff = modules.data?.find((m) => m.id === 'welcome')?.enabled === false
  const textChannels = (meta.data?.channels ?? []).filter((c) => c.type === 'text' || c.type === 'announcement')
  const roles = (meta.data?.roles ?? []).filter((r) => r.id !== guildId).sort((a, b) => b.position - a.position)
  const name = me.data?.globalName ?? me.data?.username ?? 'you'
  const vars = (channelId: string): PreviewVars => ({
    user: '@' + name,
    userName: name,
    userId: me.data?.id ?? '0',
    server: guild.data?.name ?? 'Server',
    memberCount: 42,
    channel: '#' + (textChannels.find((c) => c.id === channelId)?.name ?? 'channel'),
  })
  const invalid =
    (draft.welcomeEnabled && draft.welcomeChannelId === '0') || (draft.goodbyeEnabled && draft.goodbyeChannelId === '0')

  const channelSelect = (id: string, value: string, onChange: (v: string) => void) => (
    <Select id={id} value={value} onChange={(e) => onChange(e.target.value)} disabled={meta.isPending}>
      <option value="0">{t('welcomeSettings.chooseChannel')}</option>
      {textChannels.map((c) => (
        <option key={c.id} value={c.id}>
          #{c.name}
        </option>
      ))}
    </Select>
  )

  return (
    <div className="max-w-5xl">
      <h2 className="text-panel">{t('welcomeSettings.title')}</h2>
      <p className="mt-1 text-muted">{t('welcomeSettings.subtitle')}</p>
      {moduleOff && <Notice className="mt-4">{t('welcomeSettings.moduleOff')}</Notice>}
      <p className="mt-4 text-sm text-muted">{t('welcomeSettings.guardianNote')}</p>

      <div className="mt-6 grid gap-4">
        <Panel title={t('welcomeSettings.welcome')}>
          <SwitchRow label={t('welcomeSettings.welcomeEnabled')} checked={draft.welcomeEnabled} onCheckedChange={(v) => set('welcomeEnabled', v)} />
          {draft.welcomeEnabled && (
            <div className="grid gap-4 lg:grid-cols-[1fr_22rem]">
              <div className="grid gap-4">
                <Field label={t('welcomeSettings.channel')} htmlFor="w-channel">
                  {channelSelect('w-channel', draft.welcomeChannelId, (v) => set('welcomeChannelId', v))}
                </Field>
                <SwitchRow label={t('welcomeSettings.pingMember')} help={t('welcomeSettings.pingMemberHelp')} checked={draft.pingMember} onCheckedChange={(v) => set('pingMember', v)} />
                <MessageBuilder id="w-msg" value={draft.welcomeMessage} onChange={(d) => set('welcomeMessage', d)} variables />
              </div>
              <MessagePreview doc={renderVariables(draft.welcomeMessage, vars(draft.welcomeChannelId))} />
            </div>
          )}
        </Panel>

        <Panel title={t('welcomeSettings.dm')} help={t('welcomeSettings.dmHelp')}>
          <SwitchRow label={t('welcomeSettings.dmEnabled')} checked={draft.dmEnabled} onCheckedChange={(v) => set('dmEnabled', v)} />
          {draft.dmEnabled && (
            <div className="grid gap-4 lg:grid-cols-[1fr_22rem]">
              <MessageBuilder id="w-dm" value={draft.dmMessage} onChange={(d) => set('dmMessage', d)} variables />
              <MessagePreview doc={renderVariables(draft.dmMessage, vars(draft.welcomeChannelId))} />
            </div>
          )}
        </Panel>

        <Panel title={t('welcomeSettings.goodbye')}>
          <SwitchRow label={t('welcomeSettings.goodbyeEnabled')} checked={draft.goodbyeEnabled} onCheckedChange={(v) => set('goodbyeEnabled', v)} />
          {draft.goodbyeEnabled && (
            <div className="grid gap-4 lg:grid-cols-[1fr_22rem]">
              <div className="grid gap-4">
                <Field label={t('welcomeSettings.channel')} htmlFor="g-channel">
                  {channelSelect('g-channel', draft.goodbyeChannelId, (v) => set('goodbyeChannelId', v))}
                </Field>
                <MessageBuilder id="g-msg" value={draft.goodbyeMessage} onChange={(d) => set('goodbyeMessage', d)} variables />
              </div>
              <MessagePreview doc={renderVariables(draft.goodbyeMessage, vars(draft.goodbyeChannelId))} />
            </div>
          )}
        </Panel>

        <Panel title={t('welcomeSettings.autoRoles')} help={t('welcomeSettings.autoRolesHelp')}>
          <div className="grid gap-2 sm:grid-cols-2">
            {roles.map((r) => {
              const on = draft.autoRoleIds.includes(r.id)
              const blocked = r.managed || r.dangerous
              return (
                <label key={r.id} className={`flex items-center gap-2 text-sm ${blocked ? 'text-muted' : ''}`}>
                  <input
                    type="checkbox"
                    checked={on}
                    disabled={(blocked && !on) || (!on && draft.autoRoleIds.length >= MAX_AUTO_ROLES)}
                    onChange={() => set('autoRoleIds', on ? draft.autoRoleIds.filter((x) => x !== r.id) : [...draft.autoRoleIds, r.id])}
                  />
                  @{r.name}
                  {blocked && <span className="text-xs">({t('welcomeSettings.notAllowed')})</span>}
                </label>
              )
            })}
          </div>
        </Panel>
      </div>

      <div className="sticky bottom-0 mt-6 flex items-center gap-4 border-t border-border bg-bg py-4">
        <Button onClick={() => save.mutate(draft, { onSuccess: () => setEdits(null) })} disabled={!dirty || invalid || save.isPending}>
          {save.isPending ? t('common.saving') : t('common.save')}
        </Button>
        {save.isSuccess && !dirty && <Status state="ok" className="text-sm">{t('common.saved')}</Status>}
        <RequestError error={save.error} />
      </div>
    </div>
  )
}
