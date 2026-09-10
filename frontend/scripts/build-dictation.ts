import { spawnSync } from 'node:child_process'
import { mkdirSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

if (process.platform === 'darwin') {
  const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
  const output = resolve(root, 'resources/bin/jaz-dictation')
  mkdirSync(dirname(output), { recursive: true })
  const result = spawnSync('xcrun', [
    'swiftc', '-parse-as-library', '-O', '-target', `${process.arch === 'arm64' ? 'arm64' : 'x86_64'}-apple-macos13.0`,
    resolve(root, 'native/dictation/main.swift'),
    resolve(root, 'native/dictation/audio.swift'),
    '-Xlinker', '-sectcreate', '-Xlinker', '__TEXT', '-Xlinker', '__info_plist',
    '-Xlinker', resolve(root, 'native/dictation/Info.plist'),
    '-o', output,
  ], { stdio: 'inherit' })
  if (result.error) {
    throw result.error
  }
  if (result.status !== 0) {
    process.exit(result.status ?? 1)
  }
}
