import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type { GuardianSettings } from '../api/client'
import { useGuardianSettings, useGuildMeta, useModules, useSaveGuardianSettings } from '../api/queries'
import { Button, Card, ErrorState, Field, Notice, RequestError, Select, Spinner, Status, SwitchRow, TextArea, TextInput } from '../components/ui'

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Card className="grid gap-5">
      <h3 className="font-[560]">{title}</h3>
      {children}
    </Card>
  )
}

/** A number input that keeps typing smooth and reports integers. */
function NumberField({
  id,
  label,
  help,
  value,
  min,
  max,
  onChange,
}: {
  id: string
  label: string
  help?: string
  value: number
  min: number
  max: number
  onChange: (v: number) => void
}) {
  return (
    <Field label={label} help={help} htmlFor={id}>
      <TextInput
        id={id}
        type="number"
        inputMode="numeric"
        min={min}
        max={max}
        value={Number.isNaN(value) ? '' : value}
        onChange={(e) => onChange(e.target.value === '' ? NaN : Math.trunc(Number(e.target.value)))}
      />
    </Field>
  )
}

const lines = (s: string) =>
  s
    .split(/[\n,]/)
    .map((x) => x.trim())
    .filter(Boolean)

export function GuardianSettingsPage({ guildId }: { guildId: string }) {
  const { t } = useTranslation()
  const settings = useGuardianSettings(guildId)
  const meta = useGuildMeta(guildId)
  const modules = useModules(guildId, true)
  const save = useSaveGuardianSettings(guildId)
  // Local edits; null means "showing the saved settings". Domain lists are
  // edited as text and parsed on save.
  const [edits, setEdits] = useState<GuardianSettings | null>(null)
  const [allowText, setAllowText] = useState<string | null>(null)
  const [denyText, setDenyText] = useState<string | null>(null)

  if (settings.isError) return <ErrorState onRetry={() => void settings.refetch()} />
  if (settings.isPending) return <Spinner />
  const saved = settings.data
  const draft = edits ?? saved
  const set = <K extends keyof GuardianSettings>(k: K, v: GuardianSettings[K]) => setEdits({ ...draft, [k]: v })
  const allow = allowText ?? saved.allowDomains.join('\n')
  const deny = denyText ?? saved.denyDomains.join('\n')

  const body: GuardianSettings = { ...draft, allowDomains: lines(allow), denyDomains: lines(deny) }
  const dirty = JSON.stringify(body) !== JSON.stringify(saved)
  const invalidNumber = Object.values(body).some((v) => typeof v === 'number' && Number.isNaN(v))

  const moduleOff = modules.data?.find((m) => m.id === 'guardian')?.enabled === false
  const textChannels = (meta.data?.channels ?? []).filter((c) => c.type === 'text' || c.type === 'announcement')
  // Roles Niro can assign: not @everyone, not integration roles.
  const roles = (meta.data?.roles ?? []).filter((r) => !r.managed && r.id !== guildId).sort((a, b) => b.position - a.position)

  const roleSelect = (id: string, value: string, onChange: (v: string) => void) => (
    <Select id={id} value={value} onChange={(e) => onChange(e.target.value)} disabled={meta.isPending}>
      <option value="0">{t('guardianSettings.noRole')}</option>
      {roles.map((r) => (
        <option key={r.id} value={r.id}>
          @{r.name}
        </option>
      ))}
      {value !== '0' && !roles.some((r) => r.id === value) && <option value={value}>{value}</option>}
    </Select>
  )

  function onSave() {
    save.mutate(body, {
      onSuccess: () => {
        setEdits(null)
        setAllowText(null)
        setDenyText(null)
      },
    })
  }

  return (
    <div className="max-w-3xl">
      <h2 className="text-panel">{t('guardianSettings.title')}</h2>
      <p className="mt-1 text-muted">{t('guardianSettings.subtitle')}</p>
      {moduleOff && <Notice className="mt-4">{t('guardianSettings.moduleOff')}</Notice>}
      {meta.isError && (
        <div className="mt-4">
          <RequestError error={meta.error} />
        </div>
      )}

      <div className="mt-6 grid gap-4">
        <Section title={t('guardianSettings.sections.general')}>
          <Field label={t('guardianSettings.mode')} help={t('guardianSettings.modeHelp')} htmlFor="g-mode">
            <Select id="g-mode" value={draft.mode} onChange={(e) => set('mode', e.target.value as GuardianSettings['mode'])}>
              <option value="enforce">{t('guardianSettings.modes.enforce')}</option>
              <option value="monitor">{t('guardianSettings.modes.monitor')}</option>
            </Select>
          </Field>
          <Field label={t('guardianSettings.alertChannel')} help={t('guardianSettings.alertChannelHelp')} htmlFor="g-alert">
            <Select id="g-alert" value={draft.alertChannelId} onChange={(e) => set('alertChannelId', e.target.value)} disabled={meta.isPending}>
              <option value="0">{t('guardianSettings.useModLog')}</option>
              {textChannels.map((c) => (
                <option key={c.id} value={c.id}>
                  #{c.name}
                </option>
              ))}
              {draft.alertChannelId !== '0' && !textChannels.some((c) => c.id === draft.alertChannelId) && (
                <option value={draft.alertChannelId}>{draft.alertChannelId}</option>
              )}
            </Select>
          </Field>
        </Section>

        <Section title={t('guardianSettings.sections.compromised')}>
          <SwitchRow
            label={t('guardianSettings.burstEnabled')}
            help={t('guardianSettings.burstEnabledHelp')}
            checked={draft.burstEnabled}
            onCheckedChange={(v) => set('burstEnabled', v)}
          />
          {draft.burstEnabled && (
            <div className="grid gap-4 sm:grid-cols-2">
              <NumberField id="g-burst-ch" label={t('guardianSettings.burstChannels')} value={draft.burstChannels} min={2} max={10} onChange={(v) => set('burstChannels', v)} />
              <NumberField id="g-burst-win" label={t('guardianSettings.burstWindow')} value={draft.burstWindowSeconds} min={10} max={600} onChange={(v) => set('burstWindowSeconds', v)} />
            </div>
          )}
          <NumberField id="g-quiet" label={t('guardianSettings.quietDays')} help={t('guardianSettings.quietDaysHelp')} value={draft.quietDays} min={0} max={365} onChange={(v) => set('quietDays', v)} />
        </Section>

        <Section title={t('guardianSettings.sections.scam')}>
          <SwitchRow
            label={t('guardianSettings.scamEnabled')}
            help={t('guardianSettings.scamEnabledHelp')}
            checked={draft.scamLinksEnabled}
            onCheckedChange={(v) => set('scamLinksEnabled', v)}
          />
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label={t('guardianSettings.allowDomains')} help={t('guardianSettings.domainsHelp')} htmlFor="g-allow">
              <TextArea id="g-allow" value={allow} onChange={(e) => setAllowText(e.target.value)} placeholder="partner.example" />
            </Field>
            <Field label={t('guardianSettings.denyDomains')} help={t('guardianSettings.domainsHelp')} htmlFor="g-deny">
              <TextArea id="g-deny" value={deny} onChange={(e) => setDenyText(e.target.value)} placeholder="grabify.link" />
            </Field>
          </div>
        </Section>

        <Section title={t('guardianSettings.sections.quarantine')}>
          <Field label={t('guardianSettings.quarantineMode')} htmlFor="g-qmode">
            <Select id="g-qmode" value={draft.quarantineMode} onChange={(e) => set('quarantineMode', e.target.value as GuardianSettings['quarantineMode'])}>
              <option value="timeout">{t('guardianSettings.quarantineModes.timeout')}</option>
              <option value="role">{t('guardianSettings.quarantineModes.role')}</option>
            </Select>
          </Field>
          {draft.quarantineMode === 'role' ? (
            <Field label={t('guardianSettings.quarantineRole')} help={t('guardianSettings.quarantineRoleHelp')} htmlFor="g-qrole">
              {roleSelect('g-qrole', draft.quarantineRoleId, (v) => set('quarantineRoleId', v))}
            </Field>
          ) : (
            <NumberField id="g-qhours" label={t('guardianSettings.quarantineHours')} value={draft.quarantineHours} min={1} max={672} onChange={(v) => set('quarantineHours', v)} />
          )}
          <SwitchRow label={t('guardianSettings.dmUser')} help={t('guardianSettings.dmUserHelp')} checked={draft.dmUser} onCheckedChange={(v) => set('dmUser', v)} />
        </Section>

        <Section title={t('guardianSettings.sections.raid')}>
          <SwitchRow label={t('guardianSettings.raidEnabled')} checked={draft.raidEnabled} onCheckedChange={(v) => set('raidEnabled', v)} />
          {draft.raidEnabled && (
            <div className="grid gap-4 sm:grid-cols-3">
              <NumberField id="g-rjoins" label={t('guardianSettings.raidJoins')} value={draft.raidJoins} min={3} max={200} onChange={(v) => set('raidJoins', v)} />
              <NumberField id="g-rwin" label={t('guardianSettings.raidWindow')} value={draft.raidWindowSeconds} min={5} max={600} onChange={(v) => set('raidWindowSeconds', v)} />
              <NumberField id="g-rdur" label={t('guardianSettings.raidDuration')} value={draft.raidDurationMinutes} min={5} max={1440} onChange={(v) => set('raidDurationMinutes', v)} />
            </div>
          )}
          <SwitchRow label={t('guardianSettings.raidPauseInvites')} checked={draft.raidPauseInvites} onCheckedChange={(v) => set('raidPauseInvites', v)} />
          <SwitchRow label={t('guardianSettings.raidRaiseVerification')} checked={draft.raidRaiseVerification} onCheckedChange={(v) => set('raidRaiseVerification', v)} />
          <NumberField id="g-rslow" label={t('guardianSettings.raidSlowmode')} help={t('guardianSettings.raidSlowmodeHelp')} value={draft.raidSlowmodeSeconds} min={0} max={21600} onChange={(v) => set('raidSlowmodeSeconds', v)} />
        </Section>

        <Section title={t('guardianSettings.sections.age')}>
          <div className="grid gap-4 sm:grid-cols-2">
            <NumberField id="g-age" label={t('guardianSettings.minAge')} help={t('guardianSettings.minAgeHelp')} value={draft.minAccountAgeHours} min={0} max={8760} onChange={(v) => set('minAccountAgeHours', v)} />
            <Field label={t('guardianSettings.ageAction')} htmlFor="g-age-action">
              <Select id="g-age-action" value={draft.accountAgeAction} onChange={(e) => set('accountAgeAction', e.target.value as GuardianSettings['accountAgeAction'])}>
                <option value="alert">{t('guardianSettings.ageActions.alert')}</option>
                <option value="kick">{t('guardianSettings.ageActions.kick')}</option>
                <option value="quarantine">{t('guardianSettings.ageActions.quarantine')}</option>
              </Select>
            </Field>
          </div>
        </Section>

        <Section title={t('guardianSettings.sections.verify')}>
          <Field label={t('guardianSettings.verifyRole')} help={t('guardianSettings.verifyRoleHelp')} htmlFor="g-vrole">
            {roleSelect('g-vrole', draft.verifyRoleId, (v) => set('verifyRoleId', v))}
          </Field>
          <NumberField id="g-vage" label={t('guardianSettings.verifyMinAge')} value={draft.verifyMinAccountAgeHours} min={0} max={8760} onChange={(v) => set('verifyMinAccountAgeHours', v)} />
        </Section>

        <Section title={t('guardianSettings.sections.exempt')}>
          <p className="text-help text-muted">{t('guardianSettings.exemptHelp')}</p>
          <div className="grid gap-2 sm:grid-cols-2">
            {(meta.data?.roles ?? [])
              .filter((r) => r.id !== guildId)
              .sort((a, b) => b.position - a.position)
              .map((r) => {
                const on = draft.exemptRoleIds.includes(r.id)
                return (
                  <label key={r.id} className="flex items-center gap-2 text-sm">
                    <input
                      type="checkbox"
                      checked={on}
                      onChange={() =>
                        set('exemptRoleIds', on ? draft.exemptRoleIds.filter((x) => x !== r.id) : [...draft.exemptRoleIds, r.id])
                      }
                    />
                    @{r.name}
                  </label>
                )
              })}
          </div>
        </Section>
      </div>

      <div className="sticky bottom-0 mt-6 flex items-center gap-4 border-t border-border bg-bg py-4">
        <Button onClick={onSave} disabled={!dirty || invalidNumber || save.isPending}>
          {save.isPending ? t('common.saving') : t('common.save')}
        </Button>
        {save.isSuccess && !dirty && <Status state="ok" className="text-sm">{t('common.saved')}</Status>}
        <RequestError error={save.error} />
      </div>
    </div>
  )
}
