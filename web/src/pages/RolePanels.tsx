import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { RolePanel } from '../api/client'
import { useDeleteRolePanel, useGuildMeta, useModules, useRolePanels, useSaveRolePanel } from '../api/queries'
import { componentCount, emptyDocument, limits } from '../components/builder/document'
import { MessageBuilder } from '../components/builder/MessageBuilder'
import { MessagePreview, PreviewButton, PreviewSelect } from '../components/builder/MessagePreview'
import { Badge, Button, Card, ErrorState, Field, RequestError, Select, Spinner, TextInput } from '../components/ui'

const MAX_PANELS = 25
const MAX_ROLES = 25

function newPanel(): RolePanel {
  return {
    name: '',
    channelId: '0',
    message: emptyDocument('**Pick your roles**'),
    style: 'buttons',
    mode: 'toggle',
    roles: [],
    maxRoles: 0,
    requiredRoleId: '0',
  }
}

/** Parts the panel's own controls add (same count as the Go side). */
function reservedParts(p: RolePanel) {
  return p.style === 'select' ? 2 : Math.ceil(p.roles.length / 5) + p.roles.length
}

export function RolePanelsPage({ guildId }: { guildId: string }) {
  const { t } = useTranslation()
  const panels = useRolePanels(guildId)
  const meta = useGuildMeta(guildId)
  const modules = useModules(guildId, true)
  const remove = useDeleteRolePanel(guildId)
  const [editing, setEditing] = useState<RolePanel | null>(null)

  if (panels.isError) return <ErrorState onRetry={() => void panels.refetch()} />
  if (panels.isPending) return <Spinner />
  const moduleOff = modules.data?.find((m) => m.id === 'roles')?.enabled === false
  const channelName = (id: string) => meta.data?.channels.find((c) => c.id === id)?.name ?? id

  if (editing) return <PanelEditor guildId={guildId} initial={editing} onDone={() => setEditing(null)} />

  return (
    <div className="max-w-3xl">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-lg font-semibold">{t('rolePanels.title')}</h2>
          <p className="mt-1 text-muted">{t('rolePanels.subtitle')}</p>
        </div>
        <Button onClick={() => setEditing(newPanel())} disabled={panels.data.items.length >= MAX_PANELS}>
          {t('rolePanels.new')}
        </Button>
      </div>
      {moduleOff && <p className="mt-4 rounded-lg border border-warning/40 bg-warning/10 p-3 text-sm">{t('rolePanels.moduleOff')}</p>}
      <div className="mt-6 grid gap-3">
        {panels.data.items.length === 0 && <Card className="text-muted">{t('rolePanels.empty')}</Card>}
        {panels.data.items.map((p) => (
          <Card key={p.id} className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <p className="font-medium">{p.name}</p>
              <p className="text-sm text-muted">
                #{channelName(p.channelId)} · {t(`rolePanels.styles.${p.style}`)} · {t(`rolePanels.modes.${p.mode}`)} ·{' '}
                {t('rolePanels.roleCount', { count: p.roles.length })}
              </p>
            </div>
            <div className="flex gap-2">
              {!p.messageId && <Badge tone="warning">{t('rolePanels.notPosted')}</Badge>}
              <Button variant="secondary" onClick={() => setEditing(p)}>
                {t('common.edit')}
              </Button>
              <Button
                variant="secondary"
                onClick={() => {
                  if (p.id && window.confirm(t('rolePanels.confirmDelete', { name: p.name }))) remove.mutate(p.id)
                }}
              >
                {t('common.delete')}
              </Button>
            </div>
          </Card>
        ))}
        <RequestError error={remove.error} />
      </div>
    </div>
  )
}

function PanelEditor({ guildId, initial, onDone }: { guildId: string; initial: RolePanel; onDone: () => void }) {
  const { t } = useTranslation()
  const meta = useGuildMeta(guildId)
  const save = useSaveRolePanel(guildId)
  const [p, setP] = useState<RolePanel>(initial)
  const set = <K extends keyof RolePanel>(k: K, v: RolePanel[K]) => setP({ ...p, [k]: v })

  const textChannels = (meta.data?.channels ?? []).filter((c) => c.type === 'text' || c.type === 'announcement')
  const allRoles = (meta.data?.roles ?? []).filter((r) => r.id !== guildId).sort((a, b) => b.position - a.position)
  const assignable = allRoles.filter((r) => !r.managed && !r.dangerous)
  const blocked = allRoles.filter((r) => r.managed || r.dangerous)
  const roleName = (id: string) => allRoles.find((r) => r.id === id)?.name ?? id
  const reserved = reservedParts(p)
  const tooBig = componentCount(p.message) + reserved > limits.components
  const ready = p.name.trim() !== '' && p.channelId !== '0' && p.roles.length > 0 && p.roles.every((r) => r.roleId !== '0') && !tooBig

  const setRole = (i: number, patch: Partial<RolePanel['roles'][number]>) =>
    set(
      'roles',
      p.roles.map((r, j) => (j === i ? { ...r, ...patch } : r)),
    )

  const controls =
    p.style === 'select' ? (
      <PreviewSelect placeholder={t('rolePanels.menuPlaceholder')} />
    ) : (
      <div className="flex flex-wrap gap-2">
        {p.roles.map((r, i) => (
          <PreviewButton key={i} label={r.label || roleName(r.roleId)} emoji={r.emoji} />
        ))}
      </div>
    )

  return (
    <div className="grid max-w-5xl gap-6 lg:grid-cols-[1fr_22rem]">
      <div className="grid gap-4">
        <h2 className="text-lg font-semibold">{initial.id ? t('rolePanels.editTitle') : t('rolePanels.newTitle')}</h2>
        <Card className="grid gap-4">
          <Field label={t('rolePanels.name')} help={t('rolePanels.nameHelp')} htmlFor="rp-name">
            <TextInput id="rp-name" value={p.name} maxLength={100} onChange={(e) => set('name', e.target.value)} />
          </Field>
          <Field label={t('rolePanels.channel')} htmlFor="rp-channel">
            <Select id="rp-channel" value={p.channelId} onChange={(e) => set('channelId', e.target.value)} disabled={meta.isPending}>
              <option value="0">{t('rolePanels.chooseChannel')}</option>
              {textChannels.map((c) => (
                <option key={c.id} value={c.id}>
                  #{c.name}
                </option>
              ))}
            </Select>
          </Field>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label={t('rolePanels.style')} htmlFor="rp-style">
              <Select id="rp-style" value={p.style} onChange={(e) => set('style', e.target.value as RolePanel['style'])}>
                <option value="buttons">{t('rolePanels.styles.buttons')}</option>
                <option value="select">{t('rolePanels.styles.select')}</option>
              </Select>
            </Field>
            <Field label={t('rolePanels.mode')} help={t(`rolePanels.modeHelp.${p.mode}`)} htmlFor="rp-mode">
              <Select id="rp-mode" value={p.mode} onChange={(e) => set('mode', e.target.value as RolePanel['mode'])}>
                <option value="toggle">{t('rolePanels.modes.toggle')}</option>
                <option value="unique">{t('rolePanels.modes.unique')}</option>
                <option value="verify">{t('rolePanels.modes.verify')}</option>
              </Select>
            </Field>
          </div>
        </Card>

        <Card className="grid gap-3">
          <h3 className="font-semibold">{t('rolePanels.roles')}</h3>
          {p.roles.map((r, i) => (
            <div key={i} className="grid gap-2 sm:grid-cols-[1.2fr_1fr_4rem_auto]">
              <Select aria-label={t('rolePanels.roleN', { n: i + 1 })} value={r.roleId} onChange={(e) => setRole(i, { roleId: e.target.value })}>
                <option value="0">{t('rolePanels.chooseRole')}</option>
                {assignable.map((role) => (
                  <option key={role.id} value={role.id}>
                    @{role.name}
                  </option>
                ))}
              </Select>
              <TextInput aria-label={t('rolePanels.labelN', { n: i + 1 })} placeholder={t('rolePanels.label')} value={r.label} maxLength={80} onChange={(e) => setRole(i, { label: e.target.value })} />
              <TextInput aria-label={t('rolePanels.emojiN', { n: i + 1 })} placeholder="🎨" value={r.emoji ?? ''} onChange={(e) => setRole(i, { emoji: e.target.value || undefined })} />
              <Button type="button" variant="secondary" onClick={() => set('roles', p.roles.filter((_, j) => j !== i))}>
                {t('builder.remove')}
              </Button>
              {p.style === 'select' && (
                <TextInput
                  className="sm:col-span-4"
                  aria-label={t('rolePanels.descriptionN', { n: i + 1 })}
                  placeholder={t('rolePanels.description')}
                  value={r.description ?? ''}
                  maxLength={100}
                  onChange={(e) => setRole(i, { description: e.target.value || undefined })}
                />
              )}
            </div>
          ))}
          {p.roles.length < MAX_ROLES && (
            <Button type="button" variant="secondary" onClick={() => set('roles', [...p.roles, { roleId: '0', label: '' }])}>
              {t('rolePanels.addRole')}
            </Button>
          )}
          {blocked.length > 0 && <p className="text-xs text-muted">{t('rolePanels.blockedRoles', { list: blocked.map((r) => '@' + r.name).join(', ') })}</p>}
          <div className="grid gap-4 sm:grid-cols-2">
            {p.mode === 'toggle' && (
              <Field label={t('rolePanels.maxRoles')} help={t('rolePanels.maxRolesHelp')} htmlFor="rp-max">
                <TextInput
                  id="rp-max"
                  type="number"
                  min={0}
                  max={p.roles.length}
                  value={p.maxRoles}
                  onChange={(e) => set('maxRoles', Math.max(0, Math.trunc(Number(e.target.value) || 0)))}
                />
              </Field>
            )}
            <Field label={t('rolePanels.requiredRole')} help={t('rolePanels.requiredRoleHelp')} htmlFor="rp-required">
              <Select id="rp-required" value={p.requiredRoleId} onChange={(e) => set('requiredRoleId', e.target.value)}>
                <option value="0">{t('rolePanels.anyone')}</option>
                {allRoles.map((role) => (
                  <option key={role.id} value={role.id}>
                    @{role.name}
                  </option>
                ))}
              </Select>
            </Field>
          </div>
        </Card>

        <Card className="grid gap-3">
          <h3 className="font-semibold">{t('rolePanels.message')}</h3>
          <MessageBuilder id="rp-msg" value={p.message} onChange={(m) => set('message', m)} reserved={reserved} />
        </Card>

        <div className="sticky bottom-0 flex items-center gap-4 border-t border-border bg-bg/90 py-4 backdrop-blur">
          <Button onClick={() => save.mutate(p, { onSuccess: onDone })} disabled={!ready || save.isPending}>
            {save.isPending ? t('common.saving') : t('rolePanels.saveAndPost')}
          </Button>
          <Button variant="secondary" onClick={onDone}>
            {t('common.cancel')}
          </Button>
          <RequestError error={save.error} />
        </div>
      </div>
      <div className="lg:sticky lg:top-4 lg:self-start">
        <MessagePreview doc={p.message} controls={p.roles.length > 0 ? controls : undefined} />
      </div>
    </div>
  )
}
