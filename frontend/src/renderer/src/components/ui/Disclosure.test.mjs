import { expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { Collapse } from './Collapse'
import { DisclosureTrigger } from './DisclosureTrigger'

test('collapse keeps hidden content mounted for its first transition', () => {
  const html = renderToStaticMarkup(
    createElement(Collapse, { open: false }, createElement('span', null, 'prepared content')),
  )

  expect(html).toContain('grid-rows-[0fr]')
  expect(html).toContain('prepared content')
})

test('expanded collapse does not clip nested layout motion', () => {
  const html = renderToStaticMarkup(createElement(Collapse, { open: true }, createElement('span')))

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
