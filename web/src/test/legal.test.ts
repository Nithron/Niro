import { describe, expect, it } from 'vitest'
import privacy from '../legal/privacy.md?raw'
import terms from '../legal/terms.md?raw'

describe('legal texts', () => {
  it('name the contact addresses', () => {
    expect(privacy).toContain('privacy@nithron.com')
    expect(terms).toContain('privacy@nithron.com')
  })

  // The drafts carry [TO FILL: …] markers until the PM fills in the company
  // details. The release build must not ship them: run with RELEASE=1.
  it.runIf(process.env.RELEASE === '1')('have no fields left to fill in', () => {
    expect(privacy).not.toContain('[TO FILL')
    expect(terms).not.toContain('[TO FILL')
    expect(privacy).not.toContain('Draft for review')
    expect(terms).not.toContain('Draft for review')
  })
})
