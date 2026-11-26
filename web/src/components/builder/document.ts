import type { components } from '../../api/schema'

export type MessageDocument = components['schemas']['MessageDocument']
export type MessageBlock = components['schemas']['MessageBlock']
export type BlockType = MessageBlock['type']

/** Discord's limits, mirrored from internal/core/message (the API enforces them). */
export const limits = { components: 40, text: 4000, blocks: 20, gallery: 10, links: 5 }

export const variableNames = ['user', 'user.name', 'user.id', 'server', 'memberCount', 'channel']

export function emptyDocument(text = ''): MessageDocument {
  return { version: 1, blocks: text ? [{ type: 'text', text }] : [] }
}

export function newBlock(type: BlockType): MessageBlock {
  switch (type) {
    case 'text':
      return { type, text: '' }
    case 'section':
      return { type, text: '', thumbnail: '' }
    case 'gallery':
      return { type, images: [{ url: '' }] }
    case 'separator':
      return { type, divider: true }
    case 'links':
      return { type, links: [{ label: '', url: 'https://' }] }
  }
}

/** Components the document uses (same count as the Go side). */
export function componentCount(d: MessageDocument): number {
  let n = 1
  for (const b of d.blocks) {
    if (b.type === 'section') n += 3
    else if (b.type === 'links') n += 1 + (b.links?.length ?? 0)
    else n += 1
  }
  return n
}

export function textLength(d: MessageDocument): number {
  return d.blocks.reduce((n, b) => n + [...(b.text ?? '')].length, 0)
}

export function unknownVariables(d: MessageDocument): string[] {
  const out = new Set<string>()
  for (const b of d.blocks) {
    for (const m of (b.text ?? '').matchAll(/\{([A-Za-z][A-Za-z.]*)\}/g)) {
      if (!variableNames.includes(m[1]!)) out.add(m[1]!)
    }
  }
  return [...out]
}
