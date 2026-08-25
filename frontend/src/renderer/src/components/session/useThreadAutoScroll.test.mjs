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

test('layout scroll during send cannot cancel active bottom following', () => {
  const scrollState = createThreadScrollState()
  const viewport = { clientHeight: 600, scrollHeight: 1000, scrollTop: 400 }
  scrollState.resize(viewport)
  viewport.scrollTop = 400
  viewport.scrollHeight = 1100

  expect(scrollState.scroll(viewport)).toBe(false)
  expect(viewport.scrollTop).toBe(1100)
})

test('viewport movement without content growth pauses bottom following', () => {
  const scrollState = createThreadScrollState()
  const viewport = { clientHeight: 600, scrollHeight: 1000, scrollTop: 400 }
  scrollState.resize(viewport)
  viewport.scrollTop = 300

  expect(scrollState.scroll(viewport)).toBe(true)
  viewport.scrollHeight = 1100
  expect(scrollState.resize(viewport)).toBe(true)
  expect(viewport.scrollTop).toBe(300)
})
