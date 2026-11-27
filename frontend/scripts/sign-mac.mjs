// Signs the macOS app and its native helpers (jaz-notch, jaz-keychain) under the
// app bundle identifier, and automatically activates keychain-access-groups when a
// matching unexpired macOS provisioning profile is present.
import { execFileSync } from 'node:child_process'
import { existsSync, mkdirSync, mkdtempSync, writeFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { sign } from 'app-builder-lib/out/codeSign/macCodeSign.js'

function profileAllowsKeychainGroup(groups, target) {
  if (!Array.isArray(groups)) {
    return false
  }
  return groups.some((entry) => {
    if (typeof entry !== 'string') {
      return false
    }
    if (entry === target) {
      return true
    }
    if (entry.endsWith('.*')) {
      return target.startsWith(entry.slice(0, -1))
    }
    return false
  })
}

function readProfileKeychainEntitlements(profilePath, appId, execute) {
  const command = (file, args, input) => execute(file, args, {
    input,
    encoding: 'utf8',
    stdio: ['pipe', 'pipe', 'pipe'],
    timeout: 10_000,
  })
  const profile = command('/usr/bin/security', ['cms', '-D', '-i', profilePath])
  const entitlements = JSON.parse(command('/usr/bin/plutil', ['-extract', 'Entitlements', 'json', '-o', '-', '-'], profile))
  const expiration = command('/usr/bin/plutil', ['-extract', 'ExpirationDate', 'raw', '-o', '-', '-'], profile)
  const team = entitlements?.['com.apple.developer.team-identifier']
  const identifier = entitlements?.['com.apple.application-identifier']
  const separator = typeof identifier === 'string' ? identifier.indexOf('.') : -1
  if (typeof team !== 'string' || separator < 1 || identifier.slice(separator + 1) !== appId || !(Date.parse(expiration.trim()) > Date.now())) {
    return null
  }
  const groups = entitlements?.['keychain-access-groups']
  if (!profileAllowsKeychainGroup(groups, identifier)) {
    return null
  }
  return {
    'com.apple.application-identifier': identifier,
    'com.apple.developer.team-identifier': team,
    'keychain-access-groups': [identifier],
  }
}

export function prepareMacSigning(options, packager, execute = execFileSync, env = process.env) {
  const envProfile = env.JAZ_MAC_PROVISIONING_PROFILE?.trim()
  const provisioningProfile = options.provisioningProfile || (envProfile && existsSync(resolve(envProfile)) ? resolve(envProfile) : undefined)
  let mergedEntitlements
  if (provisioningProfile) {
    try {
      const additions = readProfileKeychainEntitlements(provisioningProfile, packager.appInfo.id, execute)
      if (additions) {
        const command = (file, args, input) => execute(file, args, {
          input,
          encoding: 'utf8',
          stdio: ['pipe', 'pipe', 'pipe'],
          timeout: 10_000,
        })
        const mainOptions = options.optionsForFile(options.app)
        const base = JSON.parse(command('/usr/bin/plutil', ['-convert', 'json', '-o', '-', mainOptions.entitlements]))
        const runs = resolve(packager.projectDir, '../runs/mac-signing')
        mkdirSync(runs, { recursive: true })
        mergedEntitlements = join(mkdtempSync(join(runs, 'app-')), 'entitlements.plist')
        writeFileSync(mergedEntitlements, command('/usr/bin/plutil', ['-convert', 'xml1', '-o', '-', '-'], JSON.stringify({
          ...base,
          ...additions,
        })), { mode: 0o600 })
      }
    } catch {
      // Leave base entitlements untouched if a local profile cannot be decoded.
    }
  }
  return {
    ...options,
    ...(provisioningProfile ? { provisioningProfile } : {}),
    optionsForFile: (file) => {
      const fileOptions = options.optionsForFile(file)
      const isApp = file === options.app
      const isKeychainHelper = file.endsWith('/Contents/MacOS/jaz-keychain')
      const isNotchHelper = file.endsWith('/Contents/MacOS/jaz-notch')
      if (isApp && mergedEntitlements) {
        return { ...fileOptions, entitlements: mergedEntitlements }
      }
      if (isKeychainHelper) {
        return {
          ...fileOptions,
          ...(mergedEntitlements ? { entitlements: mergedEntitlements } : {}),
          additionalArguments: [...(fileOptions.additionalArguments ?? []), '--identifier', packager.appInfo.id],
        }
      }
      if (isNotchHelper) {
        return {
          ...fileOptions,
          additionalArguments: [...(fileOptions.additionalArguments ?? []), '--identifier', packager.appInfo.id],
        }
      }
      return fileOptions
    },
  }
}

export default (options, packager) => sign(prepareMacSigning(options, packager))
