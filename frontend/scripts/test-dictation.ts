import { spawnSync } from 'node:child_process'
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { resolve } from 'node:path'

const temporary = mkdtempSync(resolve(tmpdir(), 'jaz-dictation-test-'))
const output = resolve(temporary, 'audio-test')
try {
  const compiled = spawnSync('xcrun', [
    'swiftc', '-parse-as-library', '-Xfrontend', '-enable-actor-data-race-checks',
    'native/dictation/audio.swift', 'native/dictation/audio.test.swift', '-o', output,
  ], { stdio: 'inherit' })
  if (compiled.error) {
    throw compiled.error
  }
  if (compiled.status !== 0) {
    throw new Error('Native dictation test did not compile.')
  }
  const result = spawnSync(output, { stdio: 'inherit' })
  if (result.error) {
    throw result.error
  }
  if (result.status !== 0) {
    throw new Error('Native dictation audio test failed.')
  }
} finally {
  rmSync(temporary, { recursive: true, force: true })
}
