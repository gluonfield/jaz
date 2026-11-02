import { spawnSync } from 'node:child_process'
import { mkdirSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')

function compile(output: string, ...args: string[]) {
  mkdirSync(resolve(root, dirname(output)), { recursive: true })
  const result = spawnSync('xcrun', [
    'swiftc', '-parse-as-library', '-O', '-target', `${process.arch === 'arm64' ? 'arm64' : 'x86_64'}-apple-macos13.0`,
    ...args,
    '-o', output,
  ], { cwd: root, stdio: 'inherit' })
  if (result.error) {
    throw result.error
  }
  if (result.status !== 0) {
    process.exit(result.status ?? 1)
  }
}

if (process.platform === 'darwin') {
  compile(
    'resources/bin/jaz-dictation',
    'native/dictation/main.swift',
    'native/dictation/audio.swift',
    '-Xlinker', '-sectcreate', '-Xlinker', '__TEXT', '-Xlinker', '__info_plist',
    '-Xlinker', 'native/dictation/Info.plist',
  )
  compile('resources/macos/jaz-notch', 'native/notch/main.swift')
}
