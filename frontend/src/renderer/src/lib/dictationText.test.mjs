import { expect, test } from 'bun:test'
import { dictationText } from './dictationText'

test('dictated text separates words while respecting existing spacing and punctuation', () => {
  for (const [text, start, end, spoken, expected] of [
    ['', 0, 0, ' Hello. ', 'Hello.'],
    ['Existing', 8, 8, 'New words.', ' New words.'],
    ['Hello world', 6, 11, 'there', 'there'],
    ['Hello, friend.', 5, 5, 'dear', ' dear'],
    ['(world)', 1, 6, 'hello', 'hello'],
    ['Hello\n', 6, 6, 'world', 'world'],
    ['Hi', 2, 2, ',', ','],
    ['🌍world', 2, 2, 'hello', ' hello '],
    ['Draft', 0, 5, ' \n ', ''],
  ]) {
    expect(dictationText(text, start, end, spoken)).toBe(expected)
  }
})
