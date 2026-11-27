import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TicketCategory, TicketPanel, TicketSettings } from '../api/client'
import {
  useDeleteTicketPanel,
  useGuildMeta,
  useModules,
  useSaveTicketPanel,
  useSaveTicketSettings,
  useTicketPanels,
  useTickets,
  useTicketSettings,
} from '../api/queries'
import { emptyDocument } from '../components/builder/document'
import { MessageBuilder } from '../components/builder/MessageBuilder'
import { MessagePreview, PreviewButton } from '../components/builder/MessagePreview'
import { Icon } from '../components/Icon'
import { Badge, Button, Empty, ErrorState, Field, Notice, Panel, RequestError, Select, Spinner, Status, SwitchRow, TextInput, useConfirm } from '../components/ui'
import { formatDateTime } from '../format'

const MAX_PANELS = 10
const MAX_CATEGORIES = 5
const MAX_QUESTIONS = 5

function newCategory(): TicketCategory {
  return {
    label: '',
    mode: 'thread',
    parentId: '0',
    staffRoleIds: [],
    questions: [],
    opening: emptyDocument('Thanks for reaching out! Staff will be with you soon.'),
    maxOpen: 1,
  }
}

function newPanel(): TicketPanel {
  return { name: '', channelId: '0', message: emptyDocument('**Need help?** Open a ticket below.'), categories: [newCategory()] }
}

export function TicketsPage({ guildId }: { guildId: string }) {
  const { t } = useTranslation()
  const panels = useTicketPanels(guildId)
  const modules = useModules(guildId, true)
  const remove = useDeleteTicketPanel(guildId)
  const [confirm, confirmDialog] = useConfirm()
  const meta = useGuildMeta(guildId)
  const [editing, setEditing] = useState<TicketPanel | null>(null)

  if (editing) return <TicketPanelEditor guildId={guildId} initial={editing} onDone={() => setEditing(null)} />
  if (panels.isError) return <ErrorState onRetry={() => void panels.refetch()} />
  if (panels.isPending) return <Spinner />
  const moduleOff = modules.data?.find((m) => m.id === 'tickets')?.enabled === false
  const channelName = (id: string) => meta.data?.channels.find((c) => c.id === id)?.name ?? id

  return (
    <div className="grid gap-6">
      <div>
        <h2 className="text-panel">{t('tickets.title')}</h2>
        <p className="mt-1 text-muted">{t('tickets.subtitle')}</p>
        {moduleOff && <Notice className="mt-4">{t('tickets.moduleOff')}</Notice>}
      </div>

      <TicketSettingsCard guildId={guildId} />

      <section className="grid gap-3">
        <div className="flex items-center justify-between gap-4">
          <h3 className="font-[560]">{t('tickets.panels')}</h3>
          <Button onClick={() => setEditing(newPanel())} disabled={panels.data.items.length >= MAX_PANELS}>
            <Icon name="add" />
            {t('tickets.newPanel')}
          </Button>
        </div>
        {panels.data.items.length === 0 && <Empty>{t('tickets.noPanels')}</Empty>}
        <ul className="divide-y divide-border border-y border-border empty:hidden">
        {panels.data.items.map((p) => (
          <li key={p.id} className="flex flex-wrap items-center justify-between gap-3 py-3">
            <div>
              <p className="font-[560]">{p.name}</p>
              <p className="flex flex-wrap items-center gap-x-2 text-sm text-muted">
                <Icon name="channel" />
                {channelName(p.channelId)} · {p.categories.map((c) => c.label).join(', ')}
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
                  if (p.id && (await confirm({ message: t('tickets.confirmDelete', { name: p.name }), action: t('tickets.deleteAction') }))) remove.mutate(p.id)
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
      </section>

      <TicketList guildId={guildId} />
    </div>
  )
}

function TicketSettingsCard({ guildId }: { guildId: string }) {
  const { t } = useTranslation()
  const settings = useTicketSettings(guildId)
  const meta = useGuildMeta(guildId)
  const save = useSaveTicketSettings(guildId)
  const [edits, setEdits] = useState<TicketSettings | null>(null)
  if (!settings.data) return null
  const draft = edits ?? settings.data
  const dirty = JSON.stringify(draft) !== JSON.stringify(settings.data)
  const textChannels = (meta.data?.channels ?? []).filter((c) => c.type === 'text' || c.type === 'announcement')
  return (
    <Panel title={t('tickets.transcripts')}>
      <p className="text-help text-muted">{t('tickets.transcriptsHelp')}</p>
      <div className="grid max-w-md gap-4">
        <Field label={t('tickets.retention')} help={t('tickets.retentionHelp')} htmlFor="t-retention">
          <TextInput
            id="t-retention"
            type="number"
            min={1}
            max={365}
            value={draft.retentionDays}
            onChange={(e) => setEdits({ ...draft, retentionDays: Math.trunc(Number(e.target.value) || 0) })}
          />
        </Field>
        <Field label={t('tickets.transcriptChannel')} htmlFor="t-transcripts">
          <Select id="t-transcripts" value={draft.transcriptChannelId} onChange={(e) => setEdits({ ...draft, transcriptChannelId: e.target.value })}>
            <option value="0">{t('tickets.noTranscriptChannel')}</option>
            {textChannels.map((c) => (
              <option key={c.id} value={c.id}>
                #{c.name}
              </option>
            ))}
          </Select>
        </Field>
      </div>
      <div className="flex items-center gap-4">
        <Button onClick={() => save.mutate(draft, { onSuccess: () => setEdits(null) })} disabled={!dirty || draft.retentionDays < 1 || draft.retentionDays > 365 || save.isPending}>
          {save.isPending ? t('common.saving') : t('common.save')}
        </Button>
        {save.isSuccess && !dirty && <Status state="ok" className="text-sm">{t('common.saved')}</Status>}
        <RequestError error={save.error} />
      </div>
    </Panel>
  )
}

function TicketList({ guildId }: { guildId: string }) {
  const { t } = useTranslation()
  const [status, setStatus] = useState<'open' | 'claimed' | 'closed' | ''>('')
  const list = useTickets(guildId, status || undefined)
  const items = list.data?.pages.flatMap((p) => p.items) ?? []
  return (
    <section className="grid gap-3">
      <div className="flex items-center justify-between gap-4">
        <h3 className="font-[560]">{t('tickets.list')}</h3>
        <Select aria-label={t('tickets.filterStatus')} value={status} onChange={(e) => setStatus(e.target.value as typeof status)} className="w-auto">
          <option value="">{t('tickets.allStatuses')}</option>
          <option value="open">{t('tickets.status.open')}</option>
          <option value="claimed">{t('tickets.status.claimed')}</option>
          <option value="closed">{t('tickets.status.closed')}</option>
        </Select>
      </div>
      {list.isError && <ErrorState onRetry={() => void list.refetch()} />}
      {list.isPending && <Spinner />}
      {!list.isPending && items.length === 0 && <Empty>{t('tickets.noTickets')}</Empty>}
      {items.length > 0 && (
        <div className="overflow-x-auto border-t border-border">
          <table className="w-full text-left text-sm">
            <thead className="font-mono text-caption text-muted">
              <tr>
                <th className="px-4 py-2">#</th>
                <th className="px-4 py-2">{t('tickets.category')}</th>
                <th className="px-4 py-2">{t('tickets.opener')}</th>
                <th className="px-4 py-2">{t('tickets.statusLabel')}</th>
                <th className="px-4 py-2">{t('tickets.opened')}</th>
                <th className="px-4 py-2">
                  <span className="sr-only">{t('tickets.transcript')}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {items.map((tk) => (
                <tr key={tk.number} className="border-t border-border">
                  <td className="px-4 py-2 font-mono">{tk.number}</td>
                  <td className="px-4 py-2">{tk.categoryLabel}</td>
                  <td className="px-4 py-2">{tk.openerName}</td>
                  <td className="px-4 py-2">
                    <Badge tone={tk.status === 'open' ? 'warning' : 'neutral'}>{t(`tickets.status.${tk.status}`)}</Badge>
                    {tk.claimedByName && <span className="ml-2 text-help text-muted">{tk.claimedByName}</span>}
                  </td>
                  <td className="px-4 py-2 font-mono text-data text-muted">{formatDateTime(tk.createdAt)}</td>
                  <td className="px-4 py-2">
                    {tk.hasTranscript && (
                      <a className="inline-flex items-center gap-1 underline underline-offset-2" href={`/api/v1/guilds/${guildId}/tickets/${tk.number}/transcript`} target="_blank" rel="noopener noreferrer">
                        <Icon name="transcript" />
                        {t('tickets.transcript')}
                        <Icon name="external-link" />
                      </a>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {list.hasNextPage && (
        <Button variant="line" onClick={() => void list.fetchNextPage()} disabled={list.isFetchingNextPage}>
          {t('common.loadMore')}
        </Button>
      )}
    </section>
  )
}


function TicketPanelEditor({ guildId, initial, onDone }: { guildId: string; initial: TicketPanel; onDone: () => void }) {
  const { t } = useTranslation()
  const meta = useGuildMeta(guildId)
  const save = useSaveTicketPanel(guildId)
  const [p, setP] = useState<TicketPanel>(initial)
  const set = <K extends keyof TicketPanel>(k: K, v: TicketPanel[K]) => setP((prev) => ({ ...prev, [k]: v }))
  const setCat = (i: number, patch: Partial<TicketCategory>) =>
    setP((prev) => ({ ...prev, categories: prev.categories.map((c, j) => (j === i ? { ...c, ...patch } : c)) }))

  const textChannels = (meta.data?.channels ?? []).filter((c) => c.type === 'text' || c.type === 'announcement')
  const categories = (meta.data?.channels ?? []).filter((c) => c.type === 'category')
  const roles = (meta.data?.roles ?? []).filter((r) => r.id !== guildId && !r.managed).sort((a, b) => b.position - a.position)
  const ready =
    p.name.trim() !== '' &&
    p.channelId !== '0' &&
    p.categories.length > 0 &&
    p.categories.every((c) => c.label.trim() !== '' && c.staffRoleIds.length > 0 && c.questions.every((q) => q.label.trim() !== ''))

  return (
    <div className="grid max-w-5xl gap-6 lg:grid-cols-[1fr_22rem]">
      <div className="grid gap-4">
        <h2 className="text-panel">{initial.id ? t('tickets.editPanel') : t('tickets.newPanelTitle')}</h2>
        <Panel title={t('tickets.panel')}>
          <Field label={t('rolePanels.name')} help={t('rolePanels.nameHelp')} htmlFor="tp-name">
            <TextInput id="tp-name" value={p.name} maxLength={100} onChange={(e) => set('name', e.target.value)} />
          </Field>
          <Field label={t('rolePanels.channel')} help={t('tickets.channelHelp')} htmlFor="tp-channel">
            <Select id="tp-channel" value={p.channelId} onChange={(e) => set('channelId', e.target.value)} disabled={meta.isPending}>
              <option value="0">{t('rolePanels.chooseChannel')}</option>
              {textChannels.map((c) => (
                <option key={c.id} value={c.id}>
                  #{c.name}
                </option>
              ))}
            </Select>
          </Field>
          <MessageBuilder id="tp-msg" value={p.message} onChange={(m) => set('message', m)} reserved={1 + p.categories.length} />
        </Panel>

        {p.categories.map((c, i) => (
          <Panel key={i} title={t('tickets.categoryN', { n: i + 1 })}>
            <div className="grid max-w-md gap-4">
              <Field label={t('tickets.buttonLabel')} htmlFor={`tc-${i}-label`}>
                <TextInput id={`tc-${i}-label`} value={c.label} maxLength={80} onChange={(e) => setCat(i, { label: e.target.value })} />
              </Field>
              <Field label={t('tickets.emoji')} htmlFor={`tc-${i}-emoji`}>
                <TextInput id={`tc-${i}-emoji`} value={c.emoji ?? ''} placeholder="🎫" onChange={(e) => setCat(i, { emoji: e.target.value || undefined })} />
              </Field>
            </div>
            <div className="grid max-w-md gap-4">
              <Field label={t('tickets.mode')} help={t(`tickets.modeHelp.${c.mode}`)} htmlFor={`tc-${i}-mode`}>
                <Select id={`tc-${i}-mode`} value={c.mode} onChange={(e) => setCat(i, { mode: e.target.value as TicketCategory['mode'] })}>
                  <option value="thread">{t('tickets.modes.thread')}</option>
                  <option value="channel">{t('tickets.modes.channel')}</option>
                </Select>
              </Field>
              {c.mode === 'channel' && (
                <Field label={t('tickets.parent')} htmlFor={`tc-${i}-parent`}>
                  <Select id={`tc-${i}-parent`} value={c.parentId} onChange={(e) => setCat(i, { parentId: e.target.value })}>
                    <option value="0">{t('tickets.noParent')}</option>
                    {categories.map((ch) => (
                      <option key={ch.id} value={ch.id}>
                        {ch.name}
                      </option>
                    ))}
                  </Select>
                </Field>
              )}
              <Field label={t('tickets.maxOpen')} htmlFor={`tc-${i}-max`}>
                <TextInput
                  id={`tc-${i}-max`}
                  type="number"
                  min={1}
                  max={5}
                  value={c.maxOpen}
                  onChange={(e) => setCat(i, { maxOpen: Math.min(5, Math.max(1, Math.trunc(Number(e.target.value) || 1))) })}
                />
              </Field>
            </div>
            <fieldset className="grid gap-2">
              <legend className="mb-2 text-sm font-medium">{t('tickets.staff')}</legend>
              <div className="grid gap-2 sm:grid-cols-2">
                {roles.map((r) => {
                  const on = c.staffRoleIds.includes(r.id)
                  return (
                    <label key={r.id} className="flex items-center gap-2 text-sm">
                      <input
                        type="checkbox"
                        checked={on}
                        onChange={() => setCat(i, { staffRoleIds: on ? c.staffRoleIds.filter((x) => x !== r.id) : [...c.staffRoleIds, r.id] })}
                      />
                      @{r.name}
                    </label>
                  )
                })}
              </div>
            </fieldset>
            <fieldset className="grid gap-2">
              <legend className="mb-2 text-sm font-medium">{t('tickets.questions')}</legend>
              {c.questions.map((q, j) => {
                const setQ = (patch: Partial<typeof q>) => setCat(i, { questions: c.questions.map((x, k) => (k === j ? { ...x, ...patch } : x)) })
                return (
                  <div key={j} className="grid gap-2 border border-border p-3">
                    <TextInput aria-label={t('tickets.questionN', { n: j + 1 })} value={q.label} maxLength={45} placeholder={t('tickets.question')} onChange={(e) => setQ({ label: e.target.value })} />
                    <TextInput aria-label={t('tickets.hintN', { n: j + 1 })} value={q.placeholder ?? ''} maxLength={100} placeholder={t('tickets.hint')} onChange={(e) => setQ({ placeholder: e.target.value || undefined })} />
                    <div className="flex flex-wrap gap-6">
                      <SwitchRow label={t('tickets.long')} checked={q.long} onCheckedChange={(v) => setQ({ long: v })} />
                      <SwitchRow label={t('tickets.required')} checked={q.required} onCheckedChange={(v) => setQ({ required: v })} />
                      <Button type="button" variant="line" onClick={() => setCat(i, { questions: c.questions.filter((_, k) => k !== j) })}>
                        <Icon name="remove" />
                {t('builder.remove')}
                      </Button>
                    </div>
                  </div>
                )
              })}
              {c.questions.length < MAX_QUESTIONS && (
                <Button type="button" variant="line" onClick={() => setCat(i, { questions: [...c.questions, { label: '', long: false, required: true }] })}>
                  <Icon name="add" />
            {t('tickets.addQuestion')}
                </Button>
              )}
            </fieldset>
            <div className="grid gap-2">
              <p className="text-sm font-medium">{t('tickets.opening')}</p>
              <MessageBuilder id={`tc-${i}-opening`} value={c.opening} onChange={(d) => setCat(i, { opening: d })} reserved={3} />
            </div>
            {p.categories.length > 1 && (
              <Button type="button" variant="line" onClick={() => setP((prev) => ({ ...prev, categories: prev.categories.filter((_, j) => j !== i) }))}>
                <Icon name="remove" />
                {t('tickets.removeCategory')}
              </Button>
            )}
          </Panel>
        ))}
        {p.categories.length < MAX_CATEGORIES && (
          <Button type="button" variant="line" onClick={() => setP((prev) => ({ ...prev, categories: [...prev.categories, newCategory()] }))}>
            <Icon name="add" />
            {t('tickets.addCategory')}
          </Button>
        )}

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
        <MessagePreview
          doc={p.message}
          controls={
            <div className="flex flex-wrap gap-2">
              {p.categories.map((c, i) => (
                <PreviewButton key={i} label={c.label || t('tickets.categoryN', { n: i + 1 })} emoji={c.emoji} tone="primary" />
              ))}
            </div>
          }
        />
      </div>
    </div>
  )
}
