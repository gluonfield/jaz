import { expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { PeekCollapse } from './PeekCollapse'

// The collapsed overflow is painted under the fade, so it must stay out of the
// tab order and accessibility tree while the toggle announces what it hides.
test('collapsed overflow peeks inert behind a toggle that names the hidden count', () => {
  const html = renderToStaticMarkup(
    createElement(PeekCollapse, { hiddenCount: 3 }, createElement('button', null, 'Hidden plugin')),
  )
  const region = html.match(/<div[^>]*\bid="([^"]+)"[^>]*>/)

  expect(region?.[0]).toContain('inert')
  expect(html).toContain('Hidden plugin')
  expect(html).toContain(`aria-controls="${region?.[1]}"`)
  expect(html).toContain('aria-expanded="false"')
  expect(html).toContain('Show 3 more')
})
