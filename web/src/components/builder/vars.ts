import type { MessageDocument } from './document'

/** Example values for the preview, rendered like the bot does. */
export type PreviewVars = { user: string; userName: string; userId: string; server: string; memberCount: number; channel: string }

/** Replaces variables in a copy of the document (for the preview only). */
export function renderVariables(d: MessageDocument, v: PreviewVars): MessageDocument {
  const values: Record<string, string> = {
    user: v.user,
    'user.name': v.userName,
    'user.id': v.userId,
    server: v.server,
    memberCount: String(v.memberCount),
    channel: v.channel,
  }
  return {
    ...d,
    blocks: d.blocks.map((b) =>
      b.text === undefined ? b : { ...b, text: b.text.replace(/\{([A-Za-z][A-Za-z.]*)\}/g, (m, name: string) => values[name] ?? m) },
    ),
  }
}
