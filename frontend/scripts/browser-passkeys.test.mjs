import { expect, test } from 'bun:test'
import { execFileSync } from 'node:child_process'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { browserPasskeyBuild } from '../electron-builder.browser-passkeys.mjs'
import { browserPasskeySigning } from './sign-browser-passkeys.mjs'
import { electron } from './test-electron.mjs'

const frontend = fileURLToPath(new URL('../', import.meta.url))
const native = process.platform === 'darwin' ? test : test.skip

native('the opt-in build rejects stock Electron before packaging', () => {
  const runtime = resolve(electron, '../../../..')
  try {
    browserPasskeyBuild({ ...process.env, JAZ_ELECTRON_DIST: runtime, JAZ_MAC_PROVISIONING_PROFILE: 'fixture.provisionprofile' })
    throw new Error('Stock Electron was accepted')
  } catch (error) {
    expect(String(error.stderr)).toContain('Runtime lacks the native browser passkey patch')
  }
}, 20_000)

native('signing adds only profile-authorized main-app entitlements and preserves helper options', () => {
  const fixture = signingFixture()
  const prepared = browserPasskeySigning(fixture.options, fixture.packager, fixture.execute)
  const main = prepared.optionsForFile(fixture.options.app)
  const entitlements = JSON.parse(execFileSync('/usr/bin/plutil', ['-convert', 'json', '-o', '-', main.entitlements], { encoding: 'utf8' }))
  expect(entitlements).toMatchObject({
    'com.apple.security.cs.allow-jit': true,
    'com.apple.developer.web-browser.public-key-credential': true,
    'com.apple.application-identifier': 'TESTTEAM12.dev.example.browser',
    'com.apple.developer.team-identifier': 'TESTTEAM12',
  })
  expect(prepared.provisioningProfile).toBe(fixture.options.provisioningProfile)
  expect(prepared.optionsForFile('helper')).toBe(fixture.helper)
  expect(main.hardenedRuntime).toBe(true)
  expect(readFileSync(fixture.helper.entitlements, 'utf8')).not.toContain('public-key-credential')
})

native.each([
  { ExpirationDate: '2000-01-01T00:00:00Z' },
  { Entitlements: { 'com.apple.developer.web-browser.public-key-credential': false } },
  { Entitlements: { 'com.apple.application-identifier': 'TESTTEAM12.dev.example.other' } },
])('signing rejects an expired, unapproved or unrelated profile: %j', (changes) => {
  const fixture = signingFixture(changes)
  expect(() => browserPasskeySigning(fixture.options, fixture.packager, fixture.execute)).toThrow('Apple-approved profile matching this app')
})

function signingFixture(changes = {}) {
  const profile = {
    ExpirationDate: '2099-01-01T00:00:00Z',
    Entitlements: {
      'com.apple.developer.web-browser.public-key-credential': true,
      'com.apple.application-identifier': 'TESTTEAM12.dev.example.browser',
      'com.apple.developer.team-identifier': 'TESTTEAM12',
      ...changes.Entitlements,
    },
    ...Object.fromEntries(Object.entries(changes).filter(([key]) => key !== 'Entitlements')),
  }
  const helper = { entitlements: resolve(frontend, 'build/entitlements.mac.plist'), hardenedRuntime: true }
  return {
    options: { app: '/fixture/Browser.app', provisioningProfile: '/fixture/browser.provisionprofile', optionsForFile: () => helper },
    packager: { projectDir: frontend, appInfo: { id: 'dev.example.browser' } },
    helper,
    execute: (file, args, options) => {
      if (file !== '/usr/bin/security') return execFileSync(file, args, options)
      const xml = execFileSync('/usr/bin/plutil', ['-convert', 'xml1', '-o', '-', '-'], { ...options, input: JSON.stringify(profile) })
      const dated = execFileSync('/usr/bin/plutil', ['-replace', 'ExpirationDate', '-date', profile.ExpirationDate, '-o', '-', '-'], { ...options, input: xml })
      return execFileSync('/usr/bin/plutil', ['-insert', 'DeveloperCertificates', '-data', 'AA==', '-o', '-', '-'], { ...options, input: dated })
    },
  }
}
