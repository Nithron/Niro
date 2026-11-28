import { readdirSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

// The Nithron identity (DESIGN-APPS.md in nithron-site): no component holds a colour value or a raw palette
// class; colours come only from the names in src/styles.css, which maps them onto the vendored tokens.
// The Discord preview is the one exception: it shows a message in Discord's own colours.

// Read from disk: Vitest does not process CSS, so a ?raw import of a stylesheet comes back empty.
// Vitest runs from web/ (its root), under jsdom, where import.meta.url is not a file URL.
const src = resolve(process.cwd(), 'src') + '/'
const read = (path: string) => readFileSync(src + path, 'utf8')
const sources = Object.fromEntries(
  readdirSync(src, { recursive: true, encoding: 'utf8' })
    .map((p) => p.split('\\').join('/'))
    .filter((p) => /\.(ts|tsx|css)$/.test(p) && !/^(test|vendor)\//.test(p) && p !== 'api/schema.d.ts')
    .map((p) => [p, read(p)]),
)
const tokens = read('vendor/nithron-tokens/line.css')
const exempt = ['components/builder/MessagePreview.tsx', 'components/builder/markdown.tsx']

const hex = /#[0-9a-fA-F]{3,8}\b/g
const hue = 'red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose|slate|gray|zinc|neutral|stone|white|black'
const utility = 'bg|text|border|border-[trblxy]|from|via|to|ring|ring-offset|outline|fill|stroke|decoration|accent|caret|divide|placeholder|shadow'
const palette = new RegExp(`(?<![\\w-])(?:${utility})-(?:${hue})(?:-\\d+)?(?:/\\d+)?(?![\\w-])`, 'g')
const gradient = /\bbg-(?:linear|radial|conic|gradient)-/g

function offences(re: RegExp) {
  const found: string[] = []
  for (const [path, text] of Object.entries(sources)) {
    if (exempt.includes(path)) continue
    for (const m of text.matchAll(re)) found.push(`${path}: ${m[0]}`)
  }
  return found
}

describe('identity', () => {
  it('reads every source file', () => {
    expect(Object.keys(sources)).toContain('styles.css')
    expect(Object.keys(sources)).toContain('components/ui.tsx')
    expect(sources['styles.css']).toContain('--fg: var(--ink)')
  })

  it('has no colour values outside the Discord preview', () => {
    expect(offences(hex)).toEqual([])
  })

  it('has no raw palette classes outside the Discord preview', () => {
    expect(offences(palette)).toEqual([])
  })

  it('has no gradients', () => {
    expect(offences(gradient)).toEqual([])
  })

  // The copy is replaced whole, never edited; its first line says which release it is.
  it('carries the tokens with their package and version', () => {
    expect(tokens.split('\n')[0]).toMatch(/^\/\* @nithron\/tokens \d+\.\d+\.\d+ dist\/line\.css /)
  })
})
