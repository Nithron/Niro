import { Link } from '@tanstack/react-router'
import { Fragment, type ReactNode, useEffect } from 'react'
import privacy from '../legal/privacy.md?raw'
import terms from '../legal/terms.md?raw'
import { Logo } from '../components/ui'

const docs = { privacy, terms } as const

/**
 * The Terms of Service and Privacy Policy, readable without logging in. The
 * texts live in src/legal/*.md and are rendered by the small Markdown reader
 * below (headings, paragraphs, lists, tables, bold, links), never as HTML.
 */
export function LegalPage({ doc }: { doc: keyof typeof docs }) {
  const blocks = parse(docs[doc])
  const h1 = blocks.find((b) => b.kind === 'h1')
  const title = h1 && 'text' in h1 ? h1.text : ''
  useEffect(() => {
    document.title = `${title} · Niro`
  }, [title])
  return (
    <div className="min-h-dvh">
      <header className="border-b border-border bg-surface/60">
        <div className="mx-auto flex h-16 max-w-3xl items-center justify-between px-4">
          <Link to="/" aria-label="Niro">
            <Logo className="h-7" />
          </Link>
          <nav className="flex gap-4 text-sm text-muted">
            <Link to="/terms" className="hover:text-fg data-[status=active]:text-fg">
              Terms
            </Link>
            <Link to="/privacy" className="hover:text-fg data-[status=active]:text-fg">
              Privacy
            </Link>
          </nav>
        </div>
      </header>
      <main className="mx-auto max-w-3xl px-4 py-10 leading-relaxed">
        {blocks.map((b, i) => (
          <Block key={i} block={b} />
        ))}
      </main>
    </div>
  )
}

type BlockNode =
  | { kind: 'h1' | 'h2' | 'p'; text: string }
  | { kind: 'ul'; items: string[] }
  | { kind: 'table'; head: string[]; rows: string[][] }

function parse(md: string): BlockNode[] {
  const out: BlockNode[] = []
  const lines = md.split('\n')
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]!.trimEnd()
    if (line === '') continue
    if (line.startsWith('# ')) out.push({ kind: 'h1', text: line.slice(2) })
    else if (line.startsWith('## ')) out.push({ kind: 'h2', text: line.slice(3) })
    else if (line.startsWith('- ')) {
      const items = [line.slice(2)]
      while (lines[i + 1]?.startsWith('- ')) items.push(lines[++i]!.slice(2).trimEnd())
      out.push({ kind: 'ul', items })
    } else if (line.startsWith('|')) {
      const cells = (l: string) => l.trim().replace(/^\||\|$/g, '').split('|').map((c) => c.trim())
      const head = cells(line)
      const rows: string[][] = []
      i++ // the |---| separator
      while (lines[i + 1]?.startsWith('|')) rows.push(cells(lines[++i]!))
      out.push({ kind: 'table', head, rows })
    } else {
      let text = line
      while (lines[i + 1] && !/^(#|- |\|)/.test(lines[i + 1]!)) text += ' ' + lines[++i]!.trim()
      out.push({ kind: 'p', text })
    }
  }
  return out
}

function Block({ block }: { block: BlockNode }) {
  switch (block.kind) {
    case 'h1':
      return <h1 className="text-3xl font-semibold">{inline(block.text)}</h1>
    case 'h2':
      return <h2 className="mt-10 text-xl font-semibold">{inline(block.text)}</h2>
    case 'p':
      return <p className="mt-4 text-fg/90">{inline(block.text)}</p>
    case 'ul':
      return (
        <ul className="mt-4 list-disc space-y-1.5 pl-6 text-fg/90">
          {block.items.map((it, i) => (
            <li key={i}>{inline(it)}</li>
          ))}
        </ul>
      )
    case 'table':
      return (
        <div className="mt-4 overflow-x-auto">
          <table className="w-full border-collapse text-left text-sm">
            <thead>
              <tr>
                {block.head.map((h, i) => (
                  <th key={i} scope="col" className="border-b border-border px-3 py-2 font-semibold">
                    {inline(h)}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {block.rows.map((r, i) => (
                <tr key={i} className="align-top">
                  {r.map((c, j) => (
                    <td key={j} className="border-b border-border/60 px-3 py-2 text-fg/90">
                      {inline(c)}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )
  }
}

/** Bold and links; everything else is plain text. Only http(s) and site-relative links are made clickable. */
function inline(text: string): ReactNode {
  const parts: ReactNode[] = []
  const re = /\*\*(.+?)\*\*|\[([^\]]+)\]\(([^)\s]+)\)/g
  let last = 0
  for (let m = re.exec(text); m; m = re.exec(text)) {
    if (m.index > last) parts.push(text.slice(last, m.index))
    if (m[1] !== undefined) parts.push(<strong key={m.index}>{m[1]}</strong>)
    else {
      const href = m[3]!
      const safe = /^https?:\/\//.test(href) || href.startsWith('/')
      parts.push(
        safe ? (
          <a key={m.index} href={href} className="text-brand underline underline-offset-2" rel={href.startsWith('/') ? undefined : 'noopener noreferrer'}>
            {m[2]}
          </a>
        ) : (
          m[2]
        ),
      )
    }
    last = m.index + m[0].length
  }
  if (last < text.length) parts.push(text.slice(last))
  return parts.map((p, i) => <Fragment key={i}>{p}</Fragment>)
}
