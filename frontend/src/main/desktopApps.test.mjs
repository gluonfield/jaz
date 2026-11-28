import { describe, expect, test } from 'bun:test'
import { execFileSync, spawnSync } from 'node:child_process'
import { existsSync, mkdtempSync, readFileSync, rmSync, symlinkSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import {
  canUseDesktopPermission,
  filterPermissionsForPolicy,
  resolveScopedFolderTarget,
  syncDesktopAppVersionRecord,
} from './desktopAppsLogic.ts'
import { appLaunchArg, createLauncher, removeLauncher, shimAppId, windowsArgs } from './desktopAppLaunchers.ts'

const APP_ID = 'abcdefghijklmnopqrstuvwxyz'
const ICON = resolve(import.meta.dir, '../assets/jaz-icon-1024.png')

function launchHost(home, overrides = {}) {
  return {
    executable: resolve(import.meta.dir, '../../node_modules/electron/dist/Electron.app/Contents/MacOS/Electron'),
    args: ['/work/jaz app'],
    folder: 'Jaz Apps',
    identity: 'dev.wins.jaz.app',
    iconDir: join(home, 'icons'),
    serverName: 'jaz-apps-test',
    writeShortcut: () => false,
    ...overrides,
  }
}

const spec = (title = 'Budget / Pro') => ({ appId: APP_ID, title, png: () => readFileSync(ICON) })

describe('Electron Desktop Apps version-locked permissions & launchers', () => {
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

  test('grants only declared permissions, never machine access to outside apps, and full access only by org policy', () => {
    const requested = ['notifications', 'tray', 'fs', 'full_access']
    const declared = ['storage:personal', 'desktop:notifications', 'desktop:fs', 'desktop:full_access']
    expect(filterPermissionsForPolicy(requested, declared, true, true)).toEqual(['notifications'])
    expect(filterPermissionsForPolicy(requested, declared, false, false)).toEqual(['notifications', 'fs'])
    expect(filterPermissionsForPolicy(requested, declared, false, true)).toEqual(['notifications', 'fs', 'full_access'])
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

  test.if(process.platform === 'darwin')('installs a macOS app as an app shim of this Jaz with its own icon', () => {
    const home = mkdtempSync(join(tmpdir(), 'jaz-launcher-'))
    try {
      const host = launchHost(home)
      const { launcherPath, codeHash } = createLauncher(spec(), host, 'darwin', home)
      expect(launcherPath).toBe(join(home, 'Applications', 'Jaz Apps', 'Budget Pro.app'))
      const info = join(launcherPath, 'Contents', 'Info.plist')
      const read = (key, format = 'raw') => execFileSync('/usr/bin/plutil', ['-extract', key, format, '-o', '-', info], { encoding: 'utf8' }).trim()
      expect(read('ElectronAppShimHostPath')).toBe(resolve(host.executable, '../../..'))
      expect(read('ElectronAppShimHostBundleIdentifier')).toBe('com.github.Electron')
      expect(read('ElectronAppShimServerName')).toBe('jaz-apps-test')
      expect(JSON.parse(read('ElectronAppShimHostArguments', 'json'))).toEqual(['/work/jaz app', '--jaz-app-shim'])
      expect(read('CFBundleIdentifier')).toBe(`dev.wins.jaz.app.${APP_ID}`)
      expect(shimAppId(launcherPath)).toBe(APP_ID)
      const loader = join(resolve(host.executable, '../..'), 'Frameworks', 'Electron Framework.framework', 'Helpers', 'app_shim_loader')
      expect(codeHash).toBe(/^CDHash=(\w+)$/m.exec(spawnSync('/usr/bin/codesign', ['-dvvv', loader], { encoding: 'utf8' }).stderr)[1])
      execFileSync('/usr/bin/iconutil', ['-c', 'iconset', join(launcherPath, 'Contents', 'Resources', 'app.icns'), '-o', join(home, 'app.iconset')])
      expect(existsSync(join(home, 'app.iconset', 'icon_512x512@2x.png'))).toBe(true)

      const second = createLauncher({ ...spec(), appId: 'zyxwvutsrqponmlkjihgfedcba' }, host, 'darwin', home)
      expect(second.launcherPath).toBe(join(home, 'Applications', 'Jaz Apps', 'Budget Pro zyxwvu.app'))
      removeLauncher(host, APP_ID, launcherPath)
      expect(existsSync(launcherPath)).toBe(false)
      expect(existsSync(join(host.iconDir, `${APP_ID}.png`))).toBe(false)
    } finally {
      rmSync(home, { recursive: true, force: true })
    }
  })

  test('adds a Windows app to the Start menu with its own taskbar identity', () => {
    const home = mkdtempSync(join(tmpdir(), 'jaz-launcher-'))
    try {
      const shortcuts = []
      const host = launchHost(home, { executable: 'C:/Jaz/Jaz.exe', writeShortcut: (path, details) => shortcuts.push({ path, details }) > 0 })
      const { launcherPath } = createLauncher(spec(), host, 'win32', home, { APPDATA: join(home, 'Roaming') })
      expect(launcherPath).toBe(join(home, 'Roaming', 'Microsoft', 'Windows', 'Start Menu', 'Programs', 'Jaz Apps', 'Budget Pro.lnk'))
      expect(shortcuts).toEqual([{ path: launcherPath, details: {
        target: 'C:/Jaz/Jaz.exe',
        args: `"/work/jaz app" --jaz-app=${APP_ID}`,
        description: 'Budget / Pro',
        icon: join(host.iconDir, `${APP_ID}.ico`),
        iconIndex: 0,
        appUserModelId: `dev.wins.jaz.app.${APP_ID}`,
      } }])
      const ico = readFileSync(join(host.iconDir, `${APP_ID}.ico`))
      expect([...ico.subarray(0, 6)]).toEqual([0, 0, 1, 0, 1, 0])
      expect(ico.subarray(22, 26).toString('latin1')).toBe('\x89PNG')
      expect(appLaunchArg(['C:/Jaz/Jaz.exe', '/work/jaz app', windowsArgs(host, APP_ID).split(' ').at(-1)])).toBe(APP_ID)
      expect(appLaunchArg(['C:/Jaz/Jaz.exe', '--jaz-app=../../etc'])).toBeNull()
    } finally {
      rmSync(home, { recursive: true, force: true })
    }
  })

  test('adds a Linux app to the applications menu under its own window class', () => {
    const home = mkdtempSync(join(tmpdir(), 'jaz-launcher-'))
    try {
      const host = launchHost(home, { executable: '/opt/Jaz/jaz' })
      const { launcherPath } = createLauncher(spec(), host, 'linux', home, { XDG_DATA_HOME: join(home, 'data') })
      expect(launcherPath).toBe(join(home, 'data', 'applications', `dev.wins.jaz.app.${APP_ID}.desktop`))
      const entry = readFileSync(launcherPath, 'utf8')
      expect(entry).toContain(`Exec="/opt/Jaz/jaz" "/work/jaz app" "--jaz-app=${APP_ID}"`)
      expect(entry).toContain(`StartupWMClass=dev.wins.jaz.app.${APP_ID}`)
      expect(entry).toContain(`Icon=${join(host.iconDir, `${APP_ID}.png`)}`)
      expect(existsSync(join(host.iconDir, `${APP_ID}.png`))).toBe(true)
    } finally {
      rmSync(home, { recursive: true, force: true })
    }
  })
})
