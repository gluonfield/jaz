import { spawnSync } from 'node:child_process'
import { mkdirSync, readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const scriptDir = dirname(fileURLToPath(import.meta.url))
const frontendDir = resolve(scriptDir, '..')
const backendDir = resolve(frontendDir, '..', 'backend')
const binaryName = process.platform === 'win32' ? 'jaz.exe' : 'jaz'
const output = resolve(frontendDir, 'resources', 'bin', binaryName)
const packageVersion = JSON.parse(readFileSync(resolve(frontendDir, 'package.json'), 'utf8')).version

mkdirSync(dirname(output), { recursive: true })

const args = ['build', '-o', output]
if (packageVersion) args.push('-ldflags', `-s -w -X main.version=v${packageVersion}`)
args.push('./cmd/jaz')

const result = spawnSync('go', args, {
  cwd: backendDir,
  stdio: 'inherit',
})

if (result.error) throw result.error
process.exit(result.status ?? 1)
