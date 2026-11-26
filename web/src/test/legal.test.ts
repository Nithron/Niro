import { describe, expect, it } from 'vitest'
import privacy from '../legal/privacy.md?raw'
import terms from '../legal/terms.md?raw'

describe('legal texts', () => {
  it('name the contact addresses', () => {
    expect(privacy).toContain('privacy@nithron.com')
    expect(terms).toContain('privacy@nithron.com')
  })

  // Placeholders must never ship: a [TO FILL: …] marker or a draft notice
  // fails the build.
  it('have no fields left to fill in', () => {
    expect(privacy).not.toContain('[TO FILL')
    expect(terms).not.toContain('[TO FILL')
    expect(privacy).not.toContain('Draft for review')
    expect(terms).not.toContain('Draft for review')
  })
})
