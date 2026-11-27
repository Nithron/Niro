import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type { MessageDocument } from './document'
import { DiscordMarkdown } from './markdown'

/**
 * A live preview that imitates Discord's Components V2 look. Images are shown
 * by URL (the browser loads them, as Discord would). controls are the
 * feature's own buttons or menus, drawn after the blocks.
 */
export function MessagePreview({ doc, controls }: { doc: MessageDocument; controls?: ReactNode }) {
  const { t } = useTranslation()
  const accent = doc.accentColor !== undefined ? '#' + doc.accentColor.toString(16).padStart(6, '0') : 'transparent'
  return (
    // Framed by a hairline and labelled: inside the frame is Discord's look, not Niro's.
    <div className="border border-border p-3">
      <p className="mb-2 font-mono text-caption text-muted">{t('builder.previewIn')}</p>
    <figure aria-label={t('builder.preview')} className="rounded-lg bg-[#313338] p-4 text-sm text-[#dbdee1]">
      <figcaption className="mb-2 flex items-center gap-2 text-xs">
        <span className="font-semibold text-white">Niro</span>
        <span className="rounded bg-[#5865f2] px-1 text-[10px] font-semibold text-white">APP</span>
      </figcaption>
      <div className="grid gap-2 rounded-lg border-l-4 bg-[#2b2d31] p-3" style={{ borderLeftColor: accent }}>
        {doc.blocks.length === 0 && !controls && <p className="text-[#949ba4]">{t('builder.emptyPreview')}</p>}
        {doc.blocks.map((b, i) => {
          switch (b.type) {
            case 'text':
              return <DiscordMarkdown key={i} text={b.text ?? ''} />
            case 'section':
              return (
                <div key={i} className="flex items-start justify-between gap-3">
                  <DiscordMarkdown text={b.text ?? ''} />
                  {b.thumbnail?.startsWith('https://') && (
                    <img src={b.thumbnail} alt="" referrerPolicy="no-referrer" className="h-20 w-20 shrink-0 rounded object-cover" />
                  )}
                </div>
              )
            case 'gallery':
              return (
                <div key={i} className="grid grid-cols-2 gap-1">
                  {(b.images ?? [])
                    .filter((img) => img.url.startsWith('https://'))
                    .map((img, j) => (
                      <img key={j} src={img.url} alt={img.description ?? ''} referrerPolicy="no-referrer" className="max-h-48 w-full rounded object-cover" />
                    ))}
                </div>
              )
            case 'separator':
              return b.divider ? <hr key={i} className={`border-[#3f4147] ${b.large ? 'my-3' : 'my-1'}`} /> : <div key={i} className={b.large ? 'h-6' : 'h-2'} />
            case 'links':
              return (
                <div key={i} className="flex flex-wrap gap-2">
                  {(b.links ?? []).map((l, j) => (
                    <PreviewButton key={j} label={`${l.label} ↗`} emoji={l.emoji} />
                  ))}
                </div>
              )
          }
        })}
        {controls}
      </div>
    </figure>
    </div>
  )
}

const tones = { primary: 'bg-[#5865f2]', secondary: 'bg-[#4e5058]', success: 'bg-[#248046]', danger: 'bg-[#da373c]' }

/** A button drawn in the preview (role panels, tickets). */
export function PreviewButton({ label, emoji, tone = 'secondary' }: { label: string; emoji?: string; tone?: keyof typeof tones }) {
  return (
    <span className={`inline-flex items-center gap-1 rounded px-3 py-1 text-white ${tones[tone]}`}>
      {emoji && <span aria-hidden>{emoji}</span>}
      {label}
    </span>
  )
}

/** A select menu drawn in the preview. */
export function PreviewSelect({ placeholder }: { placeholder: string }) {
  return <div className="rounded border border-[#1e1f22] bg-[#1e1f22] px-3 py-2 text-[#949ba4]">{placeholder} ▾</div>
}
