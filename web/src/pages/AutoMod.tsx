import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { AutoModRule, GuildMeta } from '../api/client'
import { useAutoModPresets, useAutoModRules, useDeleteAutoModRule, useGuildMeta, useSaveAutoModRule } from '../api/queries'
import { Badge, Button, Card, ErrorState, Field, RequestError, Select, Spinner, Switch, SwitchRow, TextArea, TextInput } from '../components/ui'

type Trigger = AutoModRule['trigger']
type WordList = NonNullable<AutoModRule['presets']>[number]

const triggers: Trigger[] = ['keyword', 'keyword_preset', 'mention_spam', 'spam']
const wordLists: WordList[] = ['profanity', 'sexual_content', 'slurs']
const timeoutChoices = [0, 60, 300, 600, 3600, 86400, 604800] as const

const lines = (s: string) => s.split('\n').map((l) => l.trim()).filter(Boolean)

const emptyRule = (): AutoModRule => ({ name: '', enabled: true, trigger: 'keyword', blockMessage: true, keywords: [] })

export function AutoModPage({ guildId }: { guildId: string }) {
  const { t } = useTranslation()
  const rules = useAutoModRules(guildId)
  const presets = useAutoModPresets()
  const save = useSaveAutoModRule(guildId)
  const del = useDeleteAutoModRule(guildId)
  const meta = useGuildMeta(guildId)
  const [editing, setEditing] = useState<AutoModRule | null>(null)
  const [addingPreset, setAddingPreset] = useState<string | null>(null)

  if (editing) {
    return (
      <RuleEditor
        rule={editing}
        meta={meta.data}
        saving={save.isPending}
        error={save.error}
        onCancel={() => {
          save.reset()
          setEditing(null)
        }}
        onSave={(r) => save.mutate(r, { onSuccess: () => setEditing(null) })}
      />
    )
  }

  const existingNames = new Set(rules.data?.map((r) => r.name))

  return (
    <div className="grid gap-8">
      <div>
        <h2 className="text-lg font-semibold">{t('automod.title')}</h2>
        <p className="mt-1 max-w-3xl text-muted">{t('automod.subtitle')}</p>
      </div>

      <section>
        <h3 className="font-semibold">{t('automod.presetsTitle')}</h3>
        <p className="mt-1 text-sm text-muted">{t('automod.presetsSubtitle')}</p>
        <ul className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {presets.data?.map((p) => {
            const added = existingNames.has(p.rule.name)
            return (
              <li key={p.id}>
                <Card className="flex h-full flex-col gap-3">
                  <div>
                    <p className="font-semibold">{t(`automod.presets.${p.id}.name` as never)}</p>
                    <p className="mt-1 text-sm text-muted">{t(`automod.presets.${p.id}.description` as never)}</p>
                  </div>
                  <Button
                    variant="secondary"
                    className="mt-auto"
                    disabled={added || save.isPending}
                    onClick={() => {
                      setAddingPreset(p.id)
                      save.mutate(p.rule, { onSettled: () => setAddingPreset(null) })
                    }}
                  >
                    {added ? t('automod.added') : addingPreset === p.id ? t('common.saving') : t('automod.add')}
                  </Button>
                </Card>
              </li>
            )
          })}
        </ul>
        {!editing && <div className="mt-3"><RequestError error={save.error} /></div>}
      </section>

      <section>
        <div className="flex items-center justify-between">
          <h3 className="font-semibold">{t('automod.rulesTitle')}</h3>
          <Button onClick={() => setEditing(emptyRule())}>{t('automod.newRule')}</Button>
        </div>
        <div className="mt-3">
          {rules.isPending && <Spinner />}
          {rules.isError && (
            <Card>
              <RequestError error={rules.error} />
              <Button variant="secondary" className="mt-3" onClick={() => void rules.refetch()}>
                {t('app.retry')}
              </Button>
            </Card>
          )}
          {rules.data?.length === 0 && (
            <Card>
              <p className="text-muted">{t('automod.empty')}</p>
            </Card>
          )}
          <ul className="grid gap-3">
            {rules.data?.map((r) => (
              <li key={r.id}>
                <Card className="flex flex-wrap items-center justify-between gap-4">
                  <div className="min-w-0">
                    <p className="truncate font-semibold">{r.name}</p>
                    <div className="mt-1 flex flex-wrap gap-2">
                      <Badge tone="brand">{t(`automod.triggers.${r.trigger}`)}</Badge>
                      {r.createdByBot && <Badge>{t('automod.createdByNithro')}</Badge>}
                    </div>
                  </div>
                  <div className="flex items-center gap-3">
                    <Switch
                      checked={r.enabled}
                      label={t('automod.enabled')}
                      disabled={save.isPending}
                      onCheckedChange={(enabled) => save.mutate({ ...r, enabled })}
                    />
                    <Button variant="secondary" onClick={() => setEditing(r)}>
                      {t('common.edit')}
                    </Button>
                    <Button
                      variant="ghost"
                      className="text-danger hover:text-danger"
                      disabled={del.isPending}
                      onClick={() => r.id && window.confirm(t('automod.deleteConfirm', { name: r.name })) && del.mutate(r.id)}
                    >
                      {t('common.delete')}
                    </Button>
                  </div>
                </Card>
              </li>
            ))}
          </ul>
          <div className="mt-3">
            <RequestError error={del.error} />
          </div>
        </div>
      </section>
      {meta.isError && <ErrorState message={t('errors.botUnavailable')} />}
    </div>
  )
}

function RuleEditor({
  rule,
  meta,
  saving,
  error,
  onSave,
  onCancel,
}: {
  rule: AutoModRule
  meta?: GuildMeta
  saving: boolean
  error: unknown
  onSave: (r: AutoModRule) => void
  onCancel: () => void
}) {
  const { t } = useTranslation()
  const [r, setR] = useState<AutoModRule>(rule)
  const [keywords, setKeywords] = useState((rule.keywords ?? []).join('\n'))
  const [regex, setRegex] = useState((rule.regexPatterns ?? []).join('\n'))
  const [allow, setAllow] = useState((rule.allowList ?? []).join('\n'))
  const textChannels = (meta?.channels ?? []).filter((c) => c.type === 'text' || c.type === 'announcement')
  const assignableRoles = (meta?.roles ?? []).filter((role) => !role.managed)
  const timeoutAllowed = r.trigger === 'keyword' || r.trigger === 'mention_spam'

  function toggle(list: string[] | undefined, id: string, on: boolean) {
    const set = new Set(list ?? [])
    if (on) set.add(id)
    else set.delete(id)
    return [...set]
  }

  function submit(e: React.FormEvent) {
    e.preventDefault()
    onSave({
      ...r,
      keywords: lines(keywords),
      regexPatterns: lines(regex),
      allowList: lines(allow),
      timeoutSeconds: timeoutAllowed ? r.timeoutSeconds : 0,
    })
  }

  return (
    <form onSubmit={submit} className="grid max-w-2xl gap-6">
      <h2 className="text-lg font-semibold">{rule.id ? t('automod.editRule') : t('automod.newRule')}</h2>
      <Card className="grid gap-5">
        <Field label={t('automod.form.name')} htmlFor="rule-name">
          <TextInput id="rule-name" required maxLength={100} value={r.name} onChange={(e) => setR({ ...r, name: e.target.value })} />
        </Field>
        <Field label={t('automod.form.trigger')} htmlFor="rule-trigger">
          <Select
            id="rule-trigger"
            value={r.trigger}
            disabled={Boolean(rule.id)} // Discord doesn't allow changing a rule's trigger
            onChange={(e) => setR({ ...r, trigger: e.target.value as Trigger })}
          >
            {triggers.map((tr) => (
              <option key={tr} value={tr}>
                {t(`automod.triggers.${tr}`)}
              </option>
            ))}
          </Select>
        </Field>

        {r.trigger === 'keyword' && (
          <>
            <Field label={t('automod.form.keywords')} help={t('automod.form.keywordsHelp')} htmlFor="rule-keywords">
              <TextArea id="rule-keywords" rows={6} value={keywords} onChange={(e) => setKeywords(e.target.value)} />
            </Field>
            <Field label={t('automod.form.regex')} help={t('automod.form.regexHelp')} htmlFor="rule-regex">
              <TextArea id="rule-regex" rows={3} value={regex} onChange={(e) => setRegex(e.target.value)} />
            </Field>
          </>
        )}
        {r.trigger === 'keyword_preset' && (
          <fieldset className="grid gap-2">
            <legend className="mb-1 text-sm font-medium">{t('automod.form.wordLists')}</legend>
            {wordLists.map((wl) => (
              <label key={wl} className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={r.presets?.includes(wl) ?? false}
                  onChange={(e) => setR({ ...r, presets: toggle(r.presets, wl, e.target.checked) as WordList[] })}
                />
                {t(`automod.wordLists.${wl}`)}
              </label>
            ))}
          </fieldset>
        )}
        {(r.trigger === 'keyword' || r.trigger === 'keyword_preset') && (
          <Field label={t('automod.form.allowList')} help={t('automod.form.allowListHelp')} htmlFor="rule-allow">
            <TextArea id="rule-allow" rows={3} value={allow} onChange={(e) => setAllow(e.target.value)} />
          </Field>
        )}
        {r.trigger === 'mention_spam' && (
          <>
            <Field label={t('automod.form.mentionLimit')} htmlFor="rule-mentions">
              <TextInput
                id="rule-mentions"
                type="number"
                min={1}
                max={50}
                value={r.mentionLimit ?? 5}
                onChange={(e) => setR({ ...r, mentionLimit: Number(e.target.value) })}
              />
            </Field>
            <SwitchRow
              label={t('automod.form.raidProtection')}
              checked={r.mentionRaidProtection ?? false}
              onCheckedChange={(v) => setR({ ...r, mentionRaidProtection: v })}
            />
          </>
        )}
      </Card>

      <Card className="grid gap-5">
        <h3 className="font-semibold">{t('automod.form.actions')}</h3>
        <SwitchRow label={t('automod.form.blockMessage')} checked={r.blockMessage} onCheckedChange={(v) => setR({ ...r, blockMessage: v })} />
        {r.blockMessage && (
          <Field label={t('automod.form.customMessage')} htmlFor="rule-message">
            <TextInput id="rule-message" maxLength={150} value={r.customMessage ?? ''} onChange={(e) => setR({ ...r, customMessage: e.target.value })} />
          </Field>
        )}
        <Field label={t('automod.form.alertChannel')} htmlFor="rule-alert">
          <Select id="rule-alert" value={r.alertChannelId ?? ''} onChange={(e) => setR({ ...r, alertChannelId: e.target.value || undefined })}>
            <option value="">{t('automod.form.noAlert')}</option>
            {textChannels.map((c) => (
              <option key={c.id} value={c.id}>
                #{c.name}
              </option>
            ))}
          </Select>
        </Field>
        {timeoutAllowed && (
          <Field label={t('automod.form.timeout')} htmlFor="rule-timeout">
            <Select id="rule-timeout" value={r.timeoutSeconds ?? 0} onChange={(e) => setR({ ...r, timeoutSeconds: Number(e.target.value) })}>
              {timeoutChoices.map((s) => (
                <option key={s} value={s}>
                  {s === 0 ? t('automod.form.noTimeout') : t(`automod.timeouts.${s}` as never)}
                </option>
              ))}
            </Select>
          </Field>
        )}
      </Card>

      <Card className="grid gap-4">
        <p className="text-xs text-muted">{t('automod.form.exemptHelp')}</p>
        <fieldset>
          <legend className="mb-2 text-sm font-medium">{t('automod.form.exemptRoles')}</legend>
          <div className="grid max-h-48 gap-1 overflow-y-auto sm:grid-cols-2">
            {assignableRoles.map((role) => (
              <label key={role.id} className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={r.exemptRoleIds?.includes(role.id) ?? false}
                  onChange={(e) => setR({ ...r, exemptRoleIds: toggle(r.exemptRoleIds, role.id, e.target.checked) })}
                />
                {role.name}
              </label>
            ))}
          </div>
        </fieldset>
        <fieldset>
          <legend className="mb-2 text-sm font-medium">{t('automod.form.exemptChannels')}</legend>
          <div className="grid max-h-48 gap-1 overflow-y-auto sm:grid-cols-2">
            {textChannels.map((c) => (
              <label key={c.id} className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={r.exemptChannelIds?.includes(c.id) ?? false}
                  onChange={(e) => setR({ ...r, exemptChannelIds: toggle(r.exemptChannelIds, c.id, e.target.checked) })}
                />
                #{c.name}
              </label>
            ))}
          </div>
        </fieldset>
      </Card>

      <div className="flex items-center gap-3">
        <Button type="submit" disabled={saving}>
          {saving ? t('common.saving') : t('common.save')}
        </Button>
        <Button type="button" variant="ghost" onClick={onCancel}>
          {t('common.cancel')}
        </Button>
        <RequestError error={error} />
      </div>
    </form>
  )
}
