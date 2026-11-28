import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { markRows } from '../components/NiroMark'

// The dashboard draws the mark itself; it must stay the mark the brand kit is built from.
describe('the mark', () => {
  it('matches assets/brand/mark.json', () => {
    const kit = JSON.parse(readFileSync(resolve(process.cwd(), '..', 'assets/brand/mark.json'), 'utf8')) as { rows: string[] }
    expect(markRows).toEqual(kit.rows)
  })
  it('is 16 by 16', () => {
    expect(markRows).toHaveLength(16)
    for (const row of markRows) expect(row).toMatch(/^[.#]{16}$/)
  })
})
