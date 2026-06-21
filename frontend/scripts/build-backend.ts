import { spawnSync } from 'node:child_process'
import { mkdirSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const scriptDir = dirname(fileURLToPath(import.meta.url))
const frontendDir = resolve(scriptDir, '..')
const backendDir = resolve(frontendDir, '..', 'backend')
const binaryName = process.platform === 'win32' ? 'jaz.exe' : 'jaz'
const output = resolve(frontendDir, 'resources', 'bin', binaryName)

mkdirSync(dirname(output), { recursive: true })

const result = spawnSync('go', ['build', '-o', output, './cmd/jaz'], {
  cwd: backendDir,
  stdio: 'inherit',
})

if (result.error) throw result.error
process.exit(result.status ?? 1)
