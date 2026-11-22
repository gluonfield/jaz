import { expect, test } from 'bun:test'
import { createSwipe } from './swipe'

const flick = [4, 12, 28, 46, 52, 40, 31, 24, 18, 13, 9, 6, 4, 3, 2, 1]

function steps(deltas, gap = 16) {
  const swipe = createSwipe()
  return deltas.map((delta, index) => swipe(delta, 1000 + index * gap)).filter(Boolean)
}

test('one swipe moves one step however long it coasts', () => {
  expect(steps(flick)).toEqual([1])
  expect(steps(flick.map((delta) => -delta))).toEqual([-1])
})

test('a second swipe during the coasting of the first moves again', () => {
  expect(steps([...flick.slice(0, 10), ...flick])).toEqual([1, 1])
  expect(steps([...flick, ...flick, ...flick])).toEqual([1, 1, 1])
})

test('swiping back straight away moves back', () => {
  expect(steps([...flick.slice(0, 8), ...flick.map((delta) => -delta)])).toEqual([1, -1])
})

test('a short nudge does nothing', () => {
  expect(steps([3, 6, 8, 6, 4, 2, 1])).toEqual([])
})
