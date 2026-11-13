import { expect, test } from 'bun:test'
import { parseRich, richHTML, toggleRichList, toggleRichMark, writeRich } from '@/components/canvas/richtext'

const again = (source) => writeRich(parseRich(source))

test('formatted words survive a trip through the editor model', () => {
  for (const source of [
    'Plain words',
    '**Risk** and *chance*, ~~gone~~, `code`, ***both***',
    'First paragraph\n\nSecond paragraph',
    'A line\\\nand its break',
    '**Risk**\n\n- supplier slips\n- *no* spare tool\n  - nested\n\n1. one\n2. two',
    'See [the **plan**](https://example.com/a) today',
    'snake_case and 2 \\* 3 and a \\_lone\\_ mark',
    '1\\. not a list\n\n\\# not a heading',
  ]) {
    expect(again(source)).toBe(source)
  }
})

test('other spellings of the same formatting are normalised', () => {
  expect(again('* one\n* two')).toBe('- one\n- two')
  expect(again('__bold__ and _italic_')).toBe('**bold** and *italic*')
  expect(again('soft\nwrap')).toBe('soft wrap')
  expect(again('hard  \nbreak')).toBe('hard\\\nbreak')
  expect(again('3) three\n7) seven')).toBe('1. three\n2. seven')
  expect(again('2 * 3 * 4')).toBe('2 \\* 3 \\* 4')
})

test('Markdown beyond formatted words is left to the source editor', () => {
  for (const source of ['# Heading', '> quote', '| a | b |\n| --- | --- |', 'a | b\n--- | ---', '```\ncode\n```', '- [ ] task', '![image](a.png)', 'a <b>tag</b>', '\\(x\\)', '**open', '- item\ncontinued', '    indented', '---', 'AT&amp;T']) {
    expect(parseRich(source)).toBeNull()
  }
})

test('the editable HTML nests lists and keeps marks, links and breaks', () => {
  expect(richHTML(parseRich('**A** [b](u)\\\nc\n\n- one\n  1. deep\n- two\n\n1. n'))).toBe(
    '<p><strong>A</strong> <a href="u">b</a><br>c</p><ul><li>one<ol><li>deep</li></ol></li><li>two</li></ul><ol><li>n</li></ol>',
  )
  expect(richHTML(parseRich('a < b & "c"'))).toBe('<p>a &lt; b &amp; &quot;c&quot;</p>')
  expect(richHTML([])).toBe('<p><br></p>')
})

test('marks and lists toggle over every block', () => {
  const blocks = parseRich('**Risk**\n\n- slips')
  expect(writeRich(toggleRichMark(blocks, 'bold'))).toBe('**Risk**\n\n- **slips**')
  expect(writeRich(toggleRichMark(toggleRichMark(blocks, 'bold'), 'bold'))).toBe('Risk\n\n- slips')
  expect(writeRich(toggleRichMark(blocks, 'strike'))).toBe('~~**Risk**~~\n\n- ~~slips~~')
  expect(writeRich(toggleRichList(blocks, 'ol'))).toBe('1. **Risk**\n2. slips')
  expect(writeRich(toggleRichList(toggleRichList(blocks, 'ul'), 'ul'))).toBe('**Risk**\n\nslips')
})
