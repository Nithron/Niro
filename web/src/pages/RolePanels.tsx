import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { RolePanel } from '../api/client'
import { useDeleteRolePanel, useGuildMeta, useModules, useRolePanels, useSaveRolePanel } from '../api/queries'
import { componentCount, emptyDocument, limits } from '../components/builder/document'
import { MessageBuilder } from '../components/builder/MessageBuilder'
import { MessagePreview, PreviewButton, PreviewSelect } from '../components/builder/MessagePreview'
import { Badge, Button, Card, ErrorState, Field, Notice, RequestError, Select, Spinner, Status, TextInput, useConfirm } from '../components/ui'

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
  const [confirm, confirmDialog] = useConfirm()
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
          <h2 className="text-panel">{t('rolePanels.title')}</h2>
          <p className="mt-1 text-muted">{t('rolePanels.subtitle')}</p>
        </div>
        <Button onClick={() => setEditing(newPanel())} disabled={panels.data.items.length >= MAX_PANELS}>
          {t('rolePanels.new')}
        </Button>
      </div>
      {moduleOff && <Notice className="mt-4">{t('rolePanels.moduleOff')}</Notice>}
      <div className="mt-6 grid gap-3">
        {panels.data.items.length === 0 && <p className="border-y border-border py-6 text-muted">{t('rolePanels.empty')}</p>}
        <ul className="divide-y divide-border border-y border-border">
        {panels.data.items.map((p) => (
          <li key={p.id} className="flex flex-wrap items-center justify-between gap-3 py-3">
            <div>
              <p className="font-[560]">{p.name}</p>
              <p className="text-sm text-muted">
                #{channelName(p.channelId)} · {t(`rolePanels.styles.${p.style}`)} · {t(`rolePanels.modes.${p.mode}`)} ·{' '}
                {t('rolePanels.roleCount', { count: p.roles.length })}
              </p>
            </div>
            <div className="flex items-center gap-2">
              {!p.messageId && <Badge tone="warning">{t('rolePanels.notPosted')}</Badge>}
              <Button variant="line" size="compact" onClick={() => setEditing(p)}>
                {t('common.edit')}
              </Button>
              <Button
                variant="line"
                size="compact"
                onClick={async () => {
                  if (p.id && (await confirm({ message: t('rolePanels.confirmDelete', { name: p.name }), action: t('rolePanels.deleteAction') }))) remove.mutate(p.id)
                }}
              >
                {t('common.delete')}
              </Button>
            </div>
          </li>
        ))}
        </ul>
        <RequestError error={remove.error} />
        {confirmDialog}
      </div>
    </div>
  )
}

function PanelEditor({ guildId, initial, onDone }: { guildId: string; initial: RolePanel; onDone: () => void }) {
  const { t } = useTranslation()
  const meta = useGuildMeta(guildId)
  const save = useSaveRolePanel(guildId)
  const [p, setP] = useState<RolePanel>(initial)
  const set = <K extends keyof RolePanel>(k: K, v: RolePanel[K]) => setP((prev) => ({ ...prev, [k]: v }))

  const textChannels = (meta.data?.channels ?? []).filter((c) => c.type === 'text' || c.type === 'announcement')
  const allRoles = (meta.data?.roles ?? []).filter((r) => r.id !== guildId).sort((a, b) => b.position - a.position)
  const assignable = allRoles.filter((r) => !r.managed && !r.dangerous)
  const blocked = allRoles.filter((r) => r.managed || r.dangerous)
  const roleName = (id: string) => allRoles.find((r) => r.id === id)?.name ?? id
  const reserved = reservedParts(p)
  const tooBig = componentCount(p.message) + reserved > limits.components
  const roleIds = p.roles.map((r) => r.roleId)
  const unsafe = p.roles.filter((r) => blocked.some((b) => b.id === r.roleId))
  const ready =
    p.name.trim() !== '' &&
    p.channelId !== '0' &&
    p.roles.length > 0 &&
    p.roles.every((r) => r.roleId !== '0') &&
    new Set(roleIds).size === roleIds.length &&
    unsafe.length === 0 &&
    p.maxRoles <= p.roles.length &&
    !tooBig

  const setRole = (i: number, patch: Partial<RolePanel['roles'][number]>) =>
    setP((prev) => ({ ...prev, roles: prev.roles.map((r, j) => (j === i ? { ...r, ...patch } : r)) }))
  const removeRole = (i: number) =>
    setP((prev) => {
      const next = prev.roles.filter((_, j) => j !== i)
      return { ...prev, roles: next, maxRoles: Math.min(prev.maxRoles, next.length) }
    })

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
        <h2 className="text-panel">{initial.id ? t('rolePanels.editTitle') : t('rolePanels.newTitle')}</h2>
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
          <h3 className="font-[560]">{t('rolePanels.roles')}</h3>
          {p.roles.map((r, i) => (
            <div key={i} className="grid gap-2 sm:grid-cols-[1.2fr_1fr_4rem_auto]">
              <Select aria-label={t('rolePanels.roleN', { n: i + 1 })} value={r.roleId} onChange={(e) => setRole(i, { roleId: e.target.value })}>
                <option value="0">{t('rolePanels.chooseRole')}</option>
                {assignable.map((role) => (
                  <option key={role.id} value={role.id}>
                    @{role.name}
                  </option>
                ))}
                {r.roleId !== '0' && !assignable.some((role) => role.id === r.roleId) && (
                  <option value={r.roleId} disabled>
                    @{roleName(r.roleId)} ({t('rolePanels.unavailable')})
                  </option>
                )}
              </Select>
              <TextInput aria-label={t('rolePanels.labelN', { n: i + 1 })} placeholder={t('rolePanels.label')} value={r.label} maxLength={80} onChange={(e) => setRole(i, { label: e.target.value })} />
              <TextInput aria-label={t('rolePanels.emojiN', { n: i + 1 })} placeholder="🎨" value={r.emoji ?? ''} onChange={(e) => setRole(i, { emoji: e.target.value || undefined })} />
              <Button type="button" variant="line" onClick={() => removeRole(i)}>
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
            <Button type="button" variant="line" onClick={() => setP((prev) => ({ ...prev, roles: [...prev.roles, { roleId: '0', label: '' }] }))}>
              {t('rolePanels.addRole')}
            </Button>
          )}
          {unsafe.length > 0 && (
            <p className="text-sm">
              <Status state="failed">{t('rolePanels.unsafeRoles', { list: unsafe.map((r) => '@' + roleName(r.roleId)).join(', ') })}</Status>
            </p>
          )}
          {blocked.length > 0 && <p className="text-help text-muted">{t('rolePanels.blockedRoles', { list: blocked.map((r) => '@' + r.name).join(', ') })}</p>}
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
          <h3 className="font-[560]">{t('rolePanels.message')}</h3>
          <MessageBuilder id="rp-msg" value={p.message} onChange={(m) => set('message', m)} reserved={reserved} />
        </Card>

        <div className="sticky bottom-0 flex items-center gap-4 border-t border-border bg-bg py-4">
          <Button onClick={() => save.mutate(p, { onSuccess: onDone })} disabled={!ready || save.isPending}>
            {save.isPending ? t('common.saving') : t('rolePanels.saveAndPost')}
          </Button>
          <Button variant="line" onClick={onDone}>
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
