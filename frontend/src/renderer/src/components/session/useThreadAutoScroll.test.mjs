import { expect, test } from 'bun:test'
import { applyThreadResize } from './useThreadAutoScroll'

test('thread resize preserves the viewport when bottom following is paused', () => {
  const viewport = { clientHeight: 600, scrollHeight: 1400, scrollTop: 500 }

  expect(applyThreadResize(viewport, false)).toBe(true)
  expect(viewport.scrollTop).toBe(500)
})

test('thread resize pins content growth while bottom following is active', () => {
  const viewport = { clientHeight: 600, scrollHeight: 1400, scrollTop: 500 }

  expect(applyThreadResize(viewport, true)).toBe(false)
  expect(viewport.scrollTop).toBe(1400)
})
