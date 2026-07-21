import { expect, test } from 'bun:test'
import { createThreadScrollState } from './useThreadAutoScroll'

test('opening a disclosure pauses active bottom following before content grows', () => {
  const scrollState = createThreadScrollState()
  const viewport = { clientHeight: 600, scrollHeight: 1000, scrollTop: 400 }
  scrollState.pause()
  viewport.scrollHeight = 1050

  expect(scrollState.resize(viewport)).toBe(true)
  expect(viewport.scrollTop).toBe(400)
})

test('thread resize pins content growth while bottom following is active', () => {
  const scrollState = createThreadScrollState()
  const viewport = { clientHeight: 600, scrollHeight: 1400, scrollTop: 500 }

  expect(scrollState.resize(viewport)).toBe(false)
  expect(viewport.scrollTop).toBe(1400)
})
