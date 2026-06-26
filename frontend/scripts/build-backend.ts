import { spawnSync } from 'node:child_process'
import { mkdirSync, readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const scriptDir = dirname(fileURLToPath(import.meta.url))
const frontendDir = resolve(scriptDir, '..')
const backendDir = resolve(frontendDir, '..', 'backend')
const binaryName = process.platform === 'win32' ? 'jaz.exe' : 'jaz'
const output = resolve(frontendDir, 'resources', 'bin', binaryName)
const packageJSON: { version?: unknown } = JSON.parse(readFileSync(resolve(frontendDir, 'package.json'), 'utf8'))
if (typeof packageJSON.version !== 'string' || !packageJSON.version) {
  throw new Error('frontend/package.json version is required')
}
const backendVersion = packageJSON.version.startsWith('v') ? packageJSON.version : `v${packageJSON.version}`
const requireTelegramBundle =
  process.env.CI === 'true' || process.env.JAZ_REQUIRE_TELEGRAM_BUNDLE === '1'
const telegramLDFlags = telegramBackendLDFlags(requireTelegramBundle)

mkdirSync(dirname(output), { recursive: true })

const ldflags = [`-s -w -X main.version=${backendVersion}`]
ldflags.push(...telegramLDFlags)

const args = ['build', '-o', output, '-ldflags', ldflags.join(' ')]
args.push('./cmd/jaz')

const result = spawnSync('go', args, {
  cwd: backendDir,
  stdio: 'inherit',
})

if (result.error) throw result.error
process.exit(result.status ?? 1)

function telegramBackendLDFlags(requireBundle: boolean): string[] {
  const id = process.env.JAZ_BUNDLED_TELEGRAM_APP_ID?.trim() ?? ''
  const hash = process.env.JAZ_BUNDLED_TELEGRAM_APP_HASH?.trim() ?? ''
  if (Boolean(id) !== Boolean(hash)) {
    throw new Error('JAZ_BUNDLED_TELEGRAM_APP_ID and JAZ_BUNDLED_TELEGRAM_APP_HASH must both be set')
  }
  if (requireBundle && !id) {
    throw new Error('Bundled Telegram app credentials are required for release backend builds')
  }
  if (id && !/^\d+$/.test(id)) {
    throw new Error('JAZ_BUNDLED_TELEGRAM_APP_ID must be numeric')
  }
  if (!id) return []
  return [
    `-X github.com/wins/jaz/backend/internal/connectors/telegram.bundledClientID=${id}`,
    `-X github.com/wins/jaz/backend/internal/connectors/telegram.bundledClientHash=${hash}`,
  ]
}
