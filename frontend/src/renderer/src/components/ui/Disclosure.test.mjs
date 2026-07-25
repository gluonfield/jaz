import { expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { Collapse } from './Collapse'
import { DisclosureTrigger } from './DisclosureTrigger'

// A clipped-but-present subtree still costs full style, layout, and effects. A
// transcript nests these three deep over hundreds of tool calls, so closed
// content must be absent from the tree, not merely hidden.
test('collapse leaves closed content unmounted', () => {
  const html = renderToStaticMarkup(
    createElement(Collapse, { open: false }, createElement('span', null, 'prepared content')),
  )

  expect(html).toContain('grid-rows-[0fr]')
  expect(html).not.toContain('prepared content')
})

test('expanded collapse mounts content and preserves nested layout motion', () => {
  const html = renderToStaticMarkup(
    createElement(Collapse, { open: true }, createElement('span', null, 'prepared content')),
  )

  expect(html).toContain('prepared content')
  expect(html).toContain('min-w-0')
  expect(html).toContain('overflow-visible')
})

test('disclosure trigger keeps its caret after the label', () => {
  const html = renderToStaticMarkup(
    createElement(DisclosureTrigger, {
      label: 'Worked for 12s',
      open: false,
      onClick: () => {},
    }),
  )

  expect(html.indexOf('Worked for 12s')).toBeLessThan(html.indexOf('lucide-chevron-right'))
  expect(html).toContain('aria-expanded="false"')
})
