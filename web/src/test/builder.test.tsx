import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it } from 'vitest'
import '../i18n'
import { componentCount, emptyDocument, textLength, unknownVariables, type MessageDocument } from '../components/builder/document'
import { MessageBuilder } from '../components/builder/MessageBuilder'
import { MessagePreview } from '../components/builder/MessagePreview'

function Harness({ onDoc }: { onDoc: (d: MessageDocument) => void }) {
  const [doc, setDoc] = useState(emptyDocument('Hello {user}'))
  return (
    <>
      <MessageBuilder
        id="t"
        value={doc}
        variables
        onChange={(d) => {
          setDoc(d)
          onDoc(d)
        }}
      />
      <MessagePreview doc={doc} />
    </>
  )
}

describe('message builder', () => {
  it('counts parts and text like the server', () => {
    const d: MessageDocument = {
      version: 1,
      blocks: [
        { type: 'text', text: 'héllo' },
        { type: 'section', text: 'x', thumbnail: 'https://a.org/x.png' },
        { type: 'links', links: [{ label: 'a', url: 'https://a.org' }, { label: 'b', url: 'https://b.org' }] },
      ],
    }
    expect(componentCount(d)).toBe(1 + 1 + 3 + 3)
    expect(textLength(d)).toBe(6)
    expect(unknownVariables({ version: 1, blocks: [{ type: 'text', text: '{user} {usr} {server}' }] })).toEqual(['usr'])
  })

  it('adds, edits and reorders blocks with a live preview', async () => {
    let last: MessageDocument = emptyDocument()
    render(<Harness onDoc={(d) => (last = d)} />)
    const preview = screen.getByRole('figure', { name: 'Preview' })
    expect(preview).toHaveTextContent('Hello {user}')

    await userEvent.selectOptions(screen.getByLabelText('Add a block'), 'links')
    await userEvent.type(screen.getByLabelText('Link 1 label'), 'Rules')
    await userEvent.click(screen.getAllByRole('button', { name: 'Move up' })[1]!)
    expect(last.blocks.map((b) => b.type)).toEqual(['links', 'text'])
    expect(preview).toHaveTextContent('Rules')

    const text = screen.getByLabelText('Text (Markdown)')
    await userEvent.clear(text)
    await userEvent.type(text, '**Hi** {{usr}')
    expect(screen.getByText(/Unknown variables/)).toHaveTextContent('{usr}')
    expect(screen.getByText('Hi').tagName).toBe('STRONG')

    await userEvent.click(screen.getByLabelText('Colour bar'))
    expect(last.accentColor).toBe(0x4cb8ff)
    await userEvent.click(screen.getByLabelText('Colour bar'))
    expect(last.accentColor).toBeUndefined()
  })

  it('never renders text as HTML', () => {
    render(<MessagePreview doc={{ version: 1, blocks: [{ type: 'text', text: '<img src=x onerror=alert(1)> [x](javascript:alert(1))' }] }} />)
    expect(document.querySelector('img')).toBeNull()
    expect(document.querySelector('a')).toBeNull()
    expect(screen.getByRole('figure')).toHaveTextContent('<img src=x onerror=alert(1)>')
  })
})
