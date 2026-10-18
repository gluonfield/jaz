import { describe, expect, test } from 'bun:test'
import { Editor } from '@tiptap/core'
import { markdownKit } from './markdownKit'

const open = (markdown) => new Editor({ element: null, extensions: markdownKit, content: markdown, contentType: 'markdown' })

// textAt is the document position where text starts.
function textAt(editor, text) {
  let found = -1
  editor.state.doc.descendants((node, pos) => {
    if (found < 0 && node.isText && node.text.includes(text)) found = pos + node.text.indexOf(text)
  })
  return found
}

describe('bot file editor', () => {
  const memory = [
    '# Memory',
    '',
    '- Ship queue fixes first',
    '- Read [the plan](https://example.com/plan) before merging',
    '',
    'The user is **Augustinas**; he prefers *short* answers and `bun` scripts.',
    '',
    '1. Audit',
    '2. Merge',
  ].join('\n')

  test('keeps a file it opens, unedited, as it was', () => {
    const editor = open(memory)
    expect(editor.getMarkdown()).toBe(memory)
    editor.destroy()
  })

  test('writes the toolbar\'s edits back as Markdown that opens to the same document', () => {
    const editor = open(memory)
    const word = textAt(editor, 'queue')
    editor.chain().setTextSelection({ from: word, to: word + 'queue'.length }).toggleBold().run()
    editor.chain().setTextSelection(textAt(editor, 'Memory')).toggleHeading({ level: 2 }).run()
    editor.chain().setTextSelection(textAt(editor, 'Audit')).toggleBulletList().run()
    const saved = editor.getMarkdown()
    expect(saved).toBe([
      '## Memory',
      '',
      '- Ship **queue** fixes first',
      '- Read [the plan](https://example.com/plan) before merging',
      '',
      'The user is **Augustinas**; he prefers *short* answers and `bun` scripts.',
      '',
      '- Audit',
      '- Merge',
    ].join('\n'))
    const reopened = open(saved)
    expect(reopened.getJSON()).toEqual(editor.getJSON())
    reopened.destroy()
    editor.destroy()
  })
})
