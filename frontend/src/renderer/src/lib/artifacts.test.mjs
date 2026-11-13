import { expect, test } from 'bun:test'
import { artifactErrorForDisplay, buildArtifactDocument } from './artifacts'

test('artifact iframe layout warnings stay internal while real errors remain visible', () => {
  expect(artifactErrorForDisplay('ResizeObserver loop completed with undelivered notifications.')).toBeUndefined()
  expect(artifactErrorForDisplay('ReferenceError: missingValue is not defined')).toBe('ReferenceError: missingValue is not defined')
})

test('a canvas card starts with its saved data, which cannot close the script that carries it', () => {
  const html = buildArtifactDocument({ title: 'card', code: '<p>card</p>', loadingMessages: [] }, '', { canvas: { data: { note: '</script><b>\u2028' } } })
  expect(html).toContain('window.data = {"note":"\\u003c/script>\\u003cb>\\u2028"}')
  expect(html.match(/<\/script>/g)).toHaveLength(1)
  expect(buildArtifactDocument({ title: 'card', code: '<p>card</p>', loadingMessages: [] }, '', { canvas: { data: undefined } })).toContain('window.data = null')
  expect(buildArtifactDocument({ title: 'card', code: '<p>card</p>', loadingMessages: [] }, '')).not.toContain('saveData')
})
