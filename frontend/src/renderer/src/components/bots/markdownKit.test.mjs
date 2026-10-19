import { describe, expect, test } from 'bun:test'
import { Editor } from '@tiptap/core'
import { markdownKit, savedMarkdown } from './markdownKit'

const open = (markdown) => new Editor({ element: null, extensions: markdownKit, content: markdown, contentType: 'markdown' })

// textAt is the document position where text starts.
function textAt(editor, text) {
  let found = -1
  editor.state.doc.descendants((node, pos) => {
    if (found < 0 && node.isText && node.text.includes(text)) found = pos + node.text.indexOf(text)
  })
  return found
}

// type inserts text right after the first occurrence of after.
function type(editor, after, text) {
  editor.view.dispatch(editor.state.tr.insertText(text, textAt(editor, after) + after.length))
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

  // A file as bots write it: the editor's own Markdown would escape its
  // underscores and brackets, encode <slug>, link the bare URL, put a blank
  // line after the heading and drop the one inside the list, lose the
  // checkbox and the image, and drop the final newline.
  const agents = [
    '# Agents',
    '## Tools',
    '- Start work with create_thread. [Source: setup, 2026-10-01]',
    '- Review every release. [Source: review, 2026-10-02]',
    '- Merge with git merge --no-ff jaz/<slug>; notes at https://example.com/a_b',
    '',
    '- [ ] Confirm the rollout',
    '![Flow](flow.png)',
    'Nothing merges before an audit.',
    '',
  ].join('\n')

  test('saves only the lines the person edited', () => {
    const editor = open(agents)
    expect(savedMarkdown(editor, agents)).toBe(agents)
    type(editor, 'create_thread', ' today')
    type(editor, 'audit.', ' Ever')
    const saved = savedMarkdown(editor, agents)
    expect(saved).toBe(agents.replace('create_thread. [Source: setup, 2026-10-01]', 'create\\_thread today. \\[Source: setup, 2026-10-01\\]').replace('audit.', 'audit. Ever'))
    const reopened = open(saved)
    expect(reopened.getJSON()).toEqual(editor.getJSON())
    reopened.destroy()
    editor.destroy()
  })

  test('separates a new paragraph from a list the file runs into it', () => {
    const purpose = 'Purpose: help.\n- one\n- two\n'
    const editor = open(purpose)
    editor.chain().setTextSelection(textAt(editor, 'help.') + 'help.'.length).splitBlock().run()
    editor.view.dispatch(editor.state.tr.insertText('Stay brief.'))
    expect(savedMarkdown(editor, purpose)).toBe('Purpose: help.\n\nStay brief.\n\n- one\n- two\n')
    editor.destroy()
  })

  test('keeps unrelated lines when the serializer normalizes a multiline heading', () => {
    const source = '> one\n> two\n>\n> three\n\n- Preserve send_message [source].\n'
    const editor = open(source)
    editor.chain().setTextSelection(textAt(editor, 'one')).toggleHeading({ level: 2 }).run()
    expect(savedMarkdown(editor, source)).toBe('> ## one  \n> two\n>\n> three\n\n- Preserve send_message [source].\n')
    editor.destroy()
  })

  test('writes the toolbar\'s edits back as Markdown that opens to the same document', () => {
    const editor = open(memory)
    const word = textAt(editor, 'queue')
    editor.chain().setTextSelection({ from: word, to: word + 'queue'.length }).toggleBold().run()
    editor.chain().setTextSelection(textAt(editor, 'Memory')).toggleHeading({ level: 2 }).run()
    editor.chain().setTextSelection(textAt(editor, 'Audit')).toggleBulletList().run()
    const saved = savedMarkdown(editor, memory)
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
