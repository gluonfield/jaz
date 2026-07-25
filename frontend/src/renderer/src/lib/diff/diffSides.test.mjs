import { expect, test } from 'bun:test'
import { diffSides, diffTokens } from '@/lib/diff/diffSides'
import { parseUnifiedDiff } from '@/lib/diff/parseUnifiedDiff'

const PATCH = [
  '@@ -1,3 +1,4 @@',
  ' type hello struct {',
  '-\tID  string `json:"id"`',
  '+\tID      string `json:"id"`',
  '+\tVersion string `json:"version"`',
  ' }',
  '@@ -20,1 +20,1 @@',
  '-\treturn nil',
  '+\treturn err',
].join('\n')

const hunks = parseUnifiedDiff(PATCH)
const rows = hunks.flatMap((hunk) => hunk.lines)

// Stand in for shiki: one token per line, carrying that line's own text.
const tokensFor = (lines) => lines.map((text) => [{ content: text }])

test('each side reads as its own file', () => {
  const sides = diffSides(hunks)
  expect(sides.old).toEqual(['type hello struct {', '\tID  string `json:"id"`', '}', '\treturn nil'])
  expect(sides.new).toEqual([
    'type hello struct {',
    '\tID      string `json:"id"`',
    '\tVersion string `json:"version"`',
    '}',
    '\treturn err',
  ])
})

test('every row reads back the tokens for its own text', () => {
  const sides = diffSides(hunks)
  const tokens = diffTokens(sides, tokensFor(sides.old), tokensFor(sides.new))
  expect(tokens.map((line) => line.map((token) => token.content).join(''))).toEqual(
    rows.map((row) => row.text),
  )
})

test('a side still highlighting leaves its rows unstyled instead of misaligned', () => {
  const sides = diffSides(hunks)
  expect(diffTokens(sides, null, null)).toBeNull()
  const tokens = diffTokens(sides, null, tokensFor(sides.new))
  for (const [index, row] of rows.entries()) {
    if (row.kind === 'del') expect(tokens[index]).toEqual([])
    else expect(tokens[index][0].content).toBe(row.text)
  }
})
