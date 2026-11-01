import { expect, test } from 'bun:test'
import { artifactErrorForDisplay } from './artifacts'

test('artifact iframe layout warnings stay internal while real errors remain visible', () => {
  expect(artifactErrorForDisplay('ResizeObserver loop completed with undelivered notifications.')).toBeUndefined()
  expect(artifactErrorForDisplay('ReferenceError: missingValue is not defined')).toBe('ReferenceError: missingValue is not defined')
})
