import { Fragment, type ReactNode } from 'react'

/**
 * A small, safe renderer for the Discord Markdown subset the preview needs:
 * headings, subtext (-#), quotes, lists, bold, italic, underline, strike,
 * inline code, code blocks, links and mentions. It builds React elements only;
 * no HTML from the text is ever injected.
 */
export function DiscordMarkdown({ text }: { text: string }) {
  const out: ReactNode[] = []
  const lines = text.split('\n')
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i] ?? ''
    if (line.startsWith('```')) {
      const body: string[] = []
      i++
      while (i < lines.length && !(lines[i] ?? '').startsWith('```')) body.push(lines[i++] ?? '')
      out.push(
        <pre key={i} className="my-1 overflow-x-auto rounded bg-black/30 p-2 font-mono text-xs">
          {body.join('\n')}
        </pre>,
      )
      continue
    }
    const h = /^(#{1,3}) (.*)$/.exec(line)
    if (h) {
      const size = h[1]!.length === 1 ? 'text-xl' : h[1]!.length === 2 ? 'text-lg' : 'text-base'
      out.push(
        <p key={i} className={`${size} font-bold`}>
          {inline(h[2]!)}
        </p>,
      )
      continue
    }
    if (line.startsWith('-# ')) {
      out.push(
        <p key={i} className="text-xs text-muted">
          {inline(line.slice(3))}
        </p>,
      )
      continue
    }
    if (line.startsWith('> ')) {
      out.push(
        <p key={i} className="border-l-4 border-border pl-2">
          {inline(line.slice(2))}
        </p>,
      )
      continue
    }
    const li = /^\s*[-*] (.*)$/.exec(line)
    if (li) {
      out.push(
        <p key={i} className="pl-4">
          • {inline(li[1]!)}
        </p>,
      )
      continue
    }
    out.push(<p key={i}>{line === '' ? '\u{a0}' : inline(line)}</p>)
  }
  return <div className="space-y-0.5 break-words">{out}</div>
}

type Rule = { re: RegExp; render: (m: RegExpExecArray, key: number) => ReactNode }

function Mention({ children }: { children: ReactNode }) {
  return <span className="rounded bg-[#5865f2]/30 px-0.5 text-[#c9cdfb]">{children}</span>
}

const rules: Rule[] = [
  {
    re: /`([^`]+)`/,
    render: (m, k) => (
      <code key={k} className="rounded bg-black/30 px-1 font-mono text-[0.85em]">
        {m[1]}
      </code>
    ),
  },
  { re: /\*\*(.+?)\*\*/, render: (m, k) => <strong key={k}>{inline(m[1]!)}</strong> },
  { re: /__(.+?)__/, render: (m, k) => <u key={k}>{inline(m[1]!)}</u> },
  { re: /~~(.+?)~~/, render: (m, k) => <s key={k}>{inline(m[1]!)}</s> },
  { re: /\*(.+?)\*|_(.+?)_/, render: (m, k) => <em key={k}>{inline(m[1] ?? m[2]!)}</em> },
  {
    re: /\[([^\]]+)\]\((https?:\/\/[^)\s]+)\)/,
    render: (m, k) => (
      <span key={k} className="text-[#00a8fc] underline" title={m[2]}>
        {m[1]}
      </span>
    ),
  },
  { re: /<@&(\d+)>/, render: (_m, k) => <Mention key={k}>@role</Mention> },
  { re: /<@!?(\d+)>/, render: (_m, k) => <Mention key={k}>@user</Mention> },
  { re: /<#(\d+)>/, render: (_m, k) => <Mention key={k}>#channel</Mention> },
  { re: /\{(user|user\.name|user\.id|server|memberCount|channel)\}/, render: (m, k) => <Mention key={k}>{`{${m[1]}}`}</Mention> },
]

/** Renders inline formatting by repeatedly taking the earliest match. */
function inline(text: string): ReactNode {
  const parts: ReactNode[] = []
  let rest = text
  let key = 0
  while (rest) {
    let best: { m: RegExpExecArray; rule: Rule } | null = null
    for (const rule of rules) {
      const m = rule.re.exec(rest)
      if (m && (!best || m.index < best.m.index)) best = { m, rule }
    }
    if (!best) {
      parts.push(rest)
      break
    }
    if (best.m.index > 0) parts.push(rest.slice(0, best.m.index))
    parts.push(best.rule.render(best.m, key++))
    rest = rest.slice(best.m.index + best.m[0].length)
  }
  return <Fragment>{parts}</Fragment>
}
