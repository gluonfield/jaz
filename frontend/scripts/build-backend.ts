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
const telegramClientID = process.env.JAZ_BUNDLED_TELEGRAM_APP_ID?.trim() ?? ''
const telegramClientHash = process.env.JAZ_BUNDLED_TELEGRAM_APP_HASH?.trim() ?? ''
const requireTelegramBundle =
  process.env.CI === 'true' || process.env.JAZ_REQUIRE_TELEGRAM_BUNDLE === '1'

if (Boolean(telegramClientID) !== Boolean(telegramClientHash)) {
  throw new Error('JAZ_BUNDLED_TELEGRAM_APP_ID and JAZ_BUNDLED_TELEGRAM_APP_HASH must both be set')
}
if (requireTelegramBundle && !telegramClientID) {
  throw new Error('Bundled Telegram app credentials are required for release backend builds')
}
if (telegramClientID && !/^\d+$/.test(telegramClientID)) {
  throw new Error('JAZ_BUNDLED_TELEGRAM_APP_ID must be numeric')
}

mkdirSync(dirname(output), { recursive: true })

const ldflags = [`-s -w -X main.version=${backendVersion}`]
if (telegramClientID) {
  ldflags.push(
    `-X github.com/wins/jaz/backend/internal/connectors/telegram.bundledClientID=${telegramClientID}`,
    `-X github.com/wins/jaz/backend/internal/connectors/telegram.bundledClientHash=${telegramClientHash}`,
  )
}

const args = ['build', '-o', output, '-ldflags', ldflags.join(' ')]
args.push('./cmd/jaz')

const result = spawnSync('go', args, {
  cwd: backendDir,
  stdio: 'inherit',
})

if (result.error) throw result.error
process.exit(result.status ?? 1)
