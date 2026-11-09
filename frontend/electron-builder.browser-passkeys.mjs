import { execFileSync } from 'node:child_process'
import { mkdirSync, mkdtempSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import process from 'node:process'

const frontend = dirname(fileURLToPath(import.meta.url))

export function browserPasskeyBuild(environment = process.env, execute = execFileSync) {
  const runtime = environment.JAZ_ELECTRON_DIST?.trim()
  const profile = environment.JAZ_MAC_PROVISIONING_PROFILE?.trim()
  if (process.platform !== 'darwin' || !runtime || !profile) {
    throw new Error('Browser passkeys require macOS, JAZ_ELECTRON_DIST and JAZ_MAC_PROVISIONING_PROFILE')
  }
  const runs = resolve(frontend, '../runs/mac-passkeys/build')
  mkdirSync(runs, { recursive: true })
  const output = mkdtempSync(join(runs, 'runtime-'))
  execute(join(resolve(runtime), 'Electron.app/Contents/MacOS/Electron'), [join(frontend, 'scripts/verify-browser-passkeys.mjs')], {
    env: { ...environment, JAZ_BROWSER_PASSKEYS_CHECK_DIR: output },
    stdio: ['ignore', 'pipe', 'pipe'],
    timeout: 15_000,
  })
  return {
    extends: join(frontend, 'electron-builder.yml'),
    electronVersion: '45.2.1',
    electronDist: resolve(runtime),
    mac: { provisioningProfile: resolve(profile), sign: './scripts/sign-browser-passkeys.mjs' },
  }
}

export default () => browserPasskeyBuild()
