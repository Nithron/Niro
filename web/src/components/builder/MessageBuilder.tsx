import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Icon } from '../Icon'
import { BlockMeter, Button, Field, Select, Status, TextArea, TextInput } from '../ui'
import {
  componentCount,
  limits,
  newBlock,
  textLength,
  unknownVariables,
  variableNames,
  type BlockType,
  type MessageBlock,
  type MessageDocument,
} from './document'

const blockTypes: BlockType[] = ['text', 'section', 'gallery', 'separator', 'links']

function SmallButton(props: { onClick: () => void; disabled?: boolean; label?: string; children: ReactNode }) {
  return (
    <Button type="button" variant="line" size="compact" className="text-xs" onClick={props.onClick} disabled={props.disabled} aria-label={props.label}>
      {props.children}
    </Button>
  )
}

/**
 * The shared message builder: edits a MessageDocument block by block.
 * reserved is the number of parts the feature adds (its buttons or menus),
 * counted against Discord's limits. Variables are offered when the feature
 * renders them.
 */
export function MessageBuilder({
  id,
  value,
  onChange,
  reserved = 0,
  variables = false,
}: {
  id: string
  value: MessageDocument
  onChange: (d: MessageDocument) => void
  reserved?: number
  variables?: boolean
}) {
  const { t } = useTranslation()
  const setBlocks = (blocks: MessageBlock[]) => onChange({ ...value, blocks })
  const update = (i: number, b: MessageBlock) => setBlocks(value.blocks.map((x, j) => (j === i ? b : x)))
  const move = (i: number, by: number) => {
    const j = i + by
    if (j < 0 || j >= value.blocks.length) return
    const blocks = [...value.blocks]
    ;[blocks[i], blocks[j]] = [blocks[j]!, blocks[i]!]
    setBlocks(blocks)
  }
  const used = componentCount(value) + reserved
  const chars = textLength(value)
  const unknown = variables ? unknownVariables(value) : []
  const hasColor = value.accentColor !== undefined
  const hex = '#' + (value.accentColor ?? 0x4cb8ff).toString(16).padStart(6, '0')

  return (
    <div className="grid gap-4">
      <div className="flex flex-wrap items-center gap-4 text-sm">
        <label className="flex items-center gap-2">
          <input
            type="checkbox"
            checked={hasColor}
            onChange={(e) => {
              if (e.target.checked) onChange({ ...value, accentColor: 0x4cb8ff })
              else onChange({ version: value.version, blocks: value.blocks })
            }}
          />
          {t('builder.accent')}
        </label>
        {hasColor && (
          <input
            type="color"
            aria-label={t('builder.accentColor')}
            value={hex}
            onChange={(e) => onChange({ ...value, accentColor: parseInt(e.target.value.slice(1), 16) })}
          />
        )}
        <BlockMeter value={used} max={limits.components} label={t('builder.partsMeter')}>
          {t('builder.parts', { used, max: limits.components })}
        </BlockMeter>
        <BlockMeter value={chars} max={limits.text} label={t('builder.charsMeter')}>
          {t('builder.chars', { used: chars, max: limits.text })}
        </BlockMeter>
      </div>

      {value.blocks.map((b, i) => (
        <fieldset key={i} className="grid gap-3 border border-border p-3">
          <legend className="px-1 font-mono text-caption text-muted">{t(`builder.types.${b.type}`)}</legend>
          <BlockEditor id={`${id}-${i}`} block={b} onChange={(nb) => update(i, nb)} />
          <div className="flex gap-2">
            <SmallButton onClick={() => move(i, -1)} disabled={i === 0} label={t('builder.moveUp')}>
              <Icon name="up" />
            </SmallButton>
            <SmallButton onClick={() => move(i, 1)} disabled={i === value.blocks.length - 1} label={t('builder.moveDown')}>
              <Icon name="down" />
            </SmallButton>
            <SmallButton onClick={() => setBlocks(value.blocks.filter((_, j) => j !== i))}>
                <Icon name="remove" />
                {t('builder.removeBlock')}
              </SmallButton>
          </div>
        </fieldset>
      ))}

      <div className="flex flex-wrap items-center gap-2">
        <label htmlFor={`${id}-add`} className="text-sm font-medium">
          {t('builder.add')}
        </label>
        <Select
          id={`${id}-add`}
          value=""
          disabled={value.blocks.length >= limits.blocks}
          onChange={(e) => {
            if (e.target.value) setBlocks([...value.blocks, newBlock(e.target.value as BlockType)])
          }}
          className="w-auto"
        >
          <option value="">{t('builder.choose')}</option>
          {blockTypes.map((bt) => (
            <option key={bt} value={bt}>
              {t(`builder.types.${bt}`)}
            </option>
          ))}
        </Select>
      </div>
      {variables && <p className="text-help text-muted">{t('builder.variables', { list: variableNames.map((v) => `{${v}}`).join(' ') })}</p>}
      {unknown.length > 0 && (
        <p className="text-xs">
          <Status state="warning">{t('builder.unknownVariables', { list: unknown.map((v) => `{${v}}`).join(' ') })}</Status>
        </p>
      )}
    </div>
  )
}

function BlockEditor({ id, block, onChange }: { id: string; block: MessageBlock; onChange: (b: MessageBlock) => void }) {
  const { t } = useTranslation()
  const text = (
    <Field label={t('builder.text')} htmlFor={`${id}-text`}>
      <TextArea id={`${id}-text`} value={block.text ?? ''} maxLength={4000} onChange={(e) => onChange({ ...block, text: e.target.value })} />
    </Field>
  )
  switch (block.type) {
    case 'text':
      return text
    case 'section':
      return (
        <>
          {text}
          <Field label={t('builder.thumbnail')} help={t('builder.imageHelp')} htmlFor={`${id}-thumb`}>
            <TextInput id={`${id}-thumb`} value={block.thumbnail ?? ''} placeholder="https://" onChange={(e) => onChange({ ...block, thumbnail: e.target.value })} />
          </Field>
        </>
      )
    case 'gallery': {
      const images = block.images ?? []
      const set = (j: number, patch: Partial<(typeof images)[number]>) =>
        onChange({ ...block, images: images.map((x, k) => (k === j ? { ...x, ...patch } : x)) })
      return (
        <div className="grid gap-2">
          {images.map((img, j) => (
            <div key={j} className="grid gap-2 sm:grid-cols-[2fr_1fr_auto]">
              <TextInput aria-label={t('builder.imageUrl', { n: j + 1 })} value={img.url} placeholder="https://" onChange={(e) => set(j, { url: e.target.value })} />
              <TextInput
                aria-label={t('builder.imageAlt', { n: j + 1 })}
                value={img.description ?? ''}
                placeholder={t('builder.alt')}
                onChange={(e) => set(j, { description: e.target.value })}
              />
              <SmallButton onClick={() => onChange({ ...block, images: images.filter((_, k) => k !== j) })}>
                <Icon name="remove" />
                {t('builder.remove')}
              </SmallButton>
            </div>
          ))}
          <p className="text-help text-muted">{t('builder.imageHelp')}</p>
          {images.length < limits.gallery && (
            <SmallButton onClick={() => onChange({ ...block, images: [...images, { url: '' }] })}>
                <Icon name="add" />
                {t('builder.addImage')}
              </SmallButton>
          )}
        </div>
      )
    }
    case 'separator':
      return (
        <div className="flex gap-6 text-sm">
          <label className="flex items-center gap-2">
            <input type="checkbox" checked={!!block.divider} onChange={(e) => onChange({ ...block, divider: e.target.checked })} />
            {t('builder.divider')}
          </label>
          <label className="flex items-center gap-2">
            <input type="checkbox" checked={!!block.large} onChange={(e) => onChange({ ...block, large: e.target.checked })} />
            {t('builder.large')}
          </label>
        </div>
      )
    case 'links': {
      const links = block.links ?? []
      const set = (j: number, patch: Partial<(typeof links)[number]>) =>
        onChange({ ...block, links: links.map((x, k) => (k === j ? { ...x, ...patch } : x)) })
      return (
        <div className="grid gap-2">
          {links.map((l, j) => (
            <div key={j} className="grid gap-2 sm:grid-cols-[1fr_2fr_5rem_auto]">
              <TextInput aria-label={t('builder.linkLabel', { n: j + 1 })} value={l.label} placeholder={t('builder.label')} maxLength={80} onChange={(e) => set(j, { label: e.target.value })} />
              <TextInput aria-label={t('builder.linkUrl', { n: j + 1 })} value={l.url} onChange={(e) => set(j, { url: e.target.value })} />
              <TextInput aria-label={t('builder.linkEmoji', { n: j + 1 })} value={l.emoji ?? ''} placeholder="🙂" onChange={(e) => set(j, { emoji: e.target.value })} />
              <SmallButton onClick={() => onChange({ ...block, links: links.filter((_, k) => k !== j) })}>
                <Icon name="remove" />
                {t('builder.remove')}
              </SmallButton>
            </div>
          ))}
          {links.length < limits.links && (
            <SmallButton onClick={() => onChange({ ...block, links: [...links, { label: '', url: 'https://' }] })}>
                <Icon name="add" />
                {t('builder.addLink')}
              </SmallButton>
          )}
        </div>
      )
    }
  }
}
