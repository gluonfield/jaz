import { describe, expect, test } from 'bun:test'
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, symlinkSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import {
  canUseDesktopPermission,
  createOSLauncherShim,
  extractDesktopAppDeepLinkArg,
  filterPermissionsForPolicy,
  parseDesktopAppDeepLink,
  removeOSLauncherShim,
  resolveScopedFolderTarget,
  syncDesktopAppVersionRecord,
} from './desktopAppsLogic.ts'

const APP_ID = 'abcdefghijklmnopqrstuvwxyz'

describe('Electron Desktop Apps version-locked permissions & shims', () => {
  test('parses valid jaz://app/{id} deep links and rejects malformed ones', () => {
    expect(parseDesktopAppDeepLink(`jaz://app/${APP_ID}`)).toBe(APP_ID)
    expect(parseDesktopAppDeepLink('jaz://app/short')).toBeNull()
    expect(parseDesktopAppDeepLink('https://evil.example/app/' + APP_ID)).toBeNull()
    expect(extractDesktopAppDeepLinkArg(['--flag', `jaz://app/${APP_ID}`])).toBe(APP_ID)
  })

  test('auto-pauses elevated permissions whenever bundle_sha256 changes', () => {
    const base = {
      appId: APP_ID,
      workspaceId: 'local',
      title: 'Tracker',
      kind: 'artifact',
      installedVersion: 1,
      approvedBundleSha256: 'a'.repeat(64),
      latestBundleSha256: 'a'.repeat(64),
      declaredCapabilities: ['desktop:notifications'],
      permissions: ['notifications', 'fs'],
      scopedFolders: ['/tmp'],
      permissionsPaused: false,
      isExternalOrg: false,
      installedAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    }
    expect(canUseDesktopPermission(base, 'notifications')).toBe(true)

    const updated = syncDesktopAppVersionRecord(base, 2, 'b'.repeat(64))
    expect(updated.permissionsPaused).toBe(true)
    expect(canUseDesktopPermission(updated, 'notifications')).toBe(false)
    expect(canUseDesktopPermission(updated, 'fs')).toBe(false)
  })

  test('hard-blocks Tier 3 for external apps and respects org full_access policy', () => {
    expect(filterPermissionsForPolicy(['notifications', 'fs', 'full_access'], true, true)).toEqual(['notifications'])
    expect(filterPermissionsForPolicy(['notifications', 'fs', 'full_access'], false, false)).toEqual(['notifications', 'fs'])
    expect(filterPermissionsForPolicy(['notifications', 'fs', 'full_access'], false, true)).toEqual([
      'notifications',
      'fs',
      'full_access',
    ])
  })

  test('confines scoped folder reads and rejects traversal and symlinks', () => {
    const dir = mkdtempSync(join(tmpdir(), 'jaz-app-fs-'))
    const outside = mkdtempSync(join(tmpdir(), 'jaz-app-out-'))
    try {
      const allowedFile = join(dir, 'notes.txt')
      const secretFile = join(outside, 'secret.txt')
      writeFileSync(allowedFile, 'hello')
      writeFileSync(secretFile, 'secret')
      symlinkSync(secretFile, join(dir, 'escape.txt'))
      symlinkSync(outside, join(dir, 'escape-dir'))

      const record = {
        appId: APP_ID,
        workspaceId: 'local',
        title: 'Notes',
        kind: 'artifact',
        installedVersion: 1,
        approvedBundleSha256: 'a'.repeat(64),
        latestBundleSha256: 'a'.repeat(64),
        declaredCapabilities: ['desktop:fs'],
        permissions: ['fs'],
        scopedFolders: [dir],
        permissionsPaused: false,
        isExternalOrg: false,
        installedAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
      }
      expect(resolveScopedFolderTarget(record, dir, 'notes.txt')).toContain('notes.txt')
      expect(resolveScopedFolderTarget(record, dir, 'sub/new-note.txt')).toContain('new-note.txt')
      expect(() => resolveScopedFolderTarget(record, dir, '../secret.txt')).toThrow()
      expect(() => resolveScopedFolderTarget(record, dir, 'escape.txt')).toThrow()
      expect(() => resolveScopedFolderTarget(record, dir, 'escape-dir/new-pwn.txt')).toThrow()
      expect(() => resolveScopedFolderTarget(record, outside, 'secret.txt')).toThrow()
    } finally {
      rmSync(dir, { recursive: true, force: true })
      rmSync(outside, { recursive: true, force: true })
    }
  })

  test('creates and removes macOS and Windows OS launcher shims', () => {
    const fakeHome = mkdtempSync(join(tmpdir(), 'jaz-shim-'))
    try {
      const macShim = createOSLauncherShim('Budget / Pro', APP_ID, 'darwin', fakeHome)
      expect(macShim).toBeDefined()
      expect(existsSync(join(macShim, 'Contents', 'Info.plist'))).toBe(true)
      expect(readFileSync(join(macShim, 'Contents', 'MacOS', 'launch'), 'utf8')).toContain(`jaz://app/${APP_ID}`)
      removeOSLauncherShim(macShim)
      expect(existsSync(macShim)).toBe(false)

      const winAppData = join(fakeHome, 'AppData')
      mkdirSync(winAppData, { recursive: true })
      const winShim = createOSLauncherShim('Budget Pro', APP_ID, 'win32', fakeHome, winAppData)
      expect(winShim).toBeDefined()
      expect(readFileSync(winShim, 'utf8')).toContain(`URL=jaz://app/${APP_ID}`)
      removeOSLauncherShim(winShim)
      expect(existsSync(winShim)).toBe(false)
    } finally {
      rmSync(fakeHome, { recursive: true, force: true })
    }
  })
})
