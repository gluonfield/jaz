import { describe, expect, test } from 'bun:test'
import { Editor } from '@tiptap/core'
import { savedMarkdown } from '@/lib/savedMarkdown'
import { documentKit, replaceSource } from './documentKit'

const open = (markdown) => new Editor({ element: null, extensions: documentKit, content: markdown, contentType: 'markdown' })

// sources lists the Markdown each source node holds, in document order.
function sources(editor) {
  const found = []
  editor.state.doc.descendants((node, pos) => {
    if (node.type.name.startsWith('source')) found.push({ pos, source: node.attrs.source })
  })
  return found
}

const at = (editor, start) => sources(editor).find(({ source }) => source.startsWith(start)).pos

describe('document editor', () => {
  // A report as agents write it, with everything the reader draws that the
  // editor's own Markdown would rewrite or lose.
  const report = [
    '# Route comparison',
    'Cost is \\(c = m + t\\) per part, or $$c_{max}$$ at worst.[^quote]',
    '',
    '| Route | Cost |',
    '| --- | ---: |',
    '| Press | £1 |',
    '',
    '- [ ] Confirm the press',
    '- [x] Price the blank',
    '  - [ ] Ask for a second quote',
    '',
    '1. Run the check',
    '   ```sh',
    '   make check',
    '     --strict',
    '   ```',
    '2. Read ![the plot](<plots/first run.png> "Run 1")',
    '',
    '~~~jaz-artifact Route chart',
    '<svg viewBox="0 0 10 10"></svg>',
    '~~~',
    '',
    '\\[',
    'E = mc^2',
    '\\]',
    '',
    '$$',
    'a_1 + b_2',
    '$$',
    'Nothing ships before the audit.',
    '',
    '[^quote]: The March quote, [on file](quotes/march.md).',
    '',
  ].join('\n')

  test('keeps what it cannot edit as the Markdown it was written in', () => {
    const editor = open(report)
    expect(sources(editor).map(({ source }) => source)).toEqual([
      '\\(c = m + t\\)',
      '$$c_{max}$$',
      '[^quote]',
      '| Route | Cost |\n| --- | ---: |\n| Press | £1 |',
      '```sh\nmake check\n  --strict\n```',
      '![the plot](<plots/first run.png> "Run 1")',
      '~~~jaz-artifact Route chart\n<svg viewBox="0 0 10 10"></svg>\n~~~',
      '\\[\nE = mc^2\n\\]',
      '$$\na_1 + b_2\n$$',
      '[^quote]: The March quote, [on file](quotes/march.md).',
    ])
    expect(editor.getJSON().content.filter((node) => node.type === 'taskList')).toHaveLength(1)
    expect(savedMarkdown(editor, report)).toBe(report)
    const reopened = open(editor.getMarkdown())
    expect(reopened.getJSON()).toEqual(editor.getJSON())
    reopened.destroy()
    editor.destroy()
  })

  test('saves an edit without rewriting the rest', () => {
    const editor = open(report)
    let audit = -1
    editor.state.doc.descendants((node, pos) => {
      if (node.isText && node.text.includes('audit.')) audit = pos + node.text.length
    })
    editor.view.dispatch(editor.state.tr.insertText(' Ever', audit))
    expect(savedMarkdown(editor, report)).toBe(report.replace('audit.', 'audit. Ever'))
    editor.destroy()
  })

  test('replaces a block with whatever its edited Markdown now is', () => {
    const editor = open(report)
    replaceSource(editor, at(editor, '| Route'), '| Route | Cost |\n| --- | ---: |\n| Press | £2 |\n\nPrices are *per part*.')
    replaceSource(editor, at(editor, '$$c_{max}'), '\\(c_{max}\\) and **no more**')
    replaceSource(editor, at(editor, '\\['), '')
    replaceSource(editor, at(editor, '![the plot]'), 'the plot')
    const saved = savedMarkdown(editor, report)
    expect(saved).toBe(
      report
        .replace('or $$c_{max}$$ at', 'or \\(c_{max}\\) and **no more** at')
        .replace('£1 |\n', '£2 |\n\nPrices are *per part*.\n')
        .replace('2. Read ![the plot](<plots/first run.png> "Run 1")', '2. Read the plot')
        .replace('\\[\nE = mc^2\n\\]\n\n', ''),
    )
    const reopened = open(saved)
    expect(reopened.getJSON()).toEqual(editor.getJSON())
    reopened.destroy()
    editor.destroy()
  })
})
