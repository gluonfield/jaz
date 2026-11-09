import { execFileSync } from 'node:child_process'
import { mkdirSync, mkdtempSync, writeFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import sign from './sign-mac.mjs'

export function browserPasskeySigning(options, packager, execute = execFileSync) {
  const command = (file, args, input) => execute(file, args, { input, encoding: 'utf8', stdio: ['pipe', 'pipe', 'pipe'], timeout: 10_000 })
  let entitlement
  let expiration
  try {
    const profile = command('/usr/bin/security', ['cms', '-D', '-i', options.provisioningProfile])
    entitlement = JSON.parse(command('/usr/bin/plutil', ['-extract', 'Entitlements', 'json', '-o', '-', '-'], profile))
    expiration = command('/usr/bin/plutil', ['-extract', 'ExpirationDate', 'raw', '-o', '-', '-'], profile)
  } catch {
    throw new Error('Cannot decode the Apple provisioning profile for browser passkeys')
  }
  const team = entitlement?.['com.apple.developer.team-identifier']
  const identifier = entitlement?.['com.apple.application-identifier']
  const separator = typeof identifier === 'string' ? identifier.indexOf('.') : -1
  if (entitlement?.['com.apple.developer.web-browser.public-key-credential'] !== true ||
    typeof team !== 'string' || separator < 1 || identifier.slice(separator + 1) !== packager.appInfo.id ||
    !(Date.parse(expiration.trim()) > Date.now())) {
    throw new Error('Browser passkeys require an unexpired Apple-approved profile matching this app')
  }
  const mainOptions = options.optionsForFile(options.app)
  const base = JSON.parse(command('/usr/bin/plutil', ['-convert', 'json', '-o', '-', mainOptions.entitlements]))
  const runs = resolve(packager.projectDir, '../runs/mac-passkeys/signing')
  mkdirSync(runs, { recursive: true })
  const entitlements = join(mkdtempSync(join(runs, 'app-')), 'entitlements.plist')
  writeFileSync(entitlements, command('/usr/bin/plutil', ['-convert', 'xml1', '-o', '-', '-'], JSON.stringify({
    ...base,
    'com.apple.developer.web-browser.public-key-credential': true,
    'com.apple.application-identifier': identifier,
    'com.apple.developer.team-identifier': team,
  })), { mode: 0o600 })
  return {
    ...options,
    optionsForFile: (file) => file === options.app
      ? { ...options.optionsForFile(file), entitlements }
      : options.optionsForFile(file),
  }
}

export default (options, packager) => sign(browserPasskeySigning(options, packager), packager)
