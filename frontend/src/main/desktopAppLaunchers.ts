import { spawnSync } from 'node:child_process'
import { existsSync, mkdirSync, readFileSync, renameSync, rmSync, writeFileSync } from 'node:fs'
import { homedir } from 'node:os'
import { join, resolve } from 'node:path'
import type { ShortcutDetails } from 'electron'
import { VALID_APP_ID_RE } from './desktopAppsLogic'

// An installed app is a launcher the OS lists like any other program: a macOS
// app bundle built from Electron's app shim loader, a Start menu shortcut on
// Windows and a desktop entry on Linux. Each starts this copy of Jaz, which
// opens the app in a window of its own.

const APP_ARG = '--jaz-app='
// Passed by a macOS launcher that had to start Jaz before connecting to it.
export const SHIM_LAUNCH_ARG = '--jaz-app-shim'

export interface LaunchHost {
  // The executable and arguments that start this copy of Jaz.
  executable: string
  args: string[]
  // The folder launchers are installed into, such as "Jaz Apps".
  folder: string
  // The reverse-DNS prefix of each app's bundle, taskbar and desktop entry identity.
  identity: string
  // Where each app's icon is kept for its windows and for rebuilding its launcher.
  iconDir: string
  // The app shim server a macOS launcher connects to.
  serverName: string
  writeShortcut: (path: string, details: ShortcutDetails) => boolean
}

export interface LauncherSpec {
  appId: string
  title: string
  // The app's icon as a square PNG of the given size.
  png: (size: number) => Buffer
}

export interface Launcher {
  launcherPath?: string
  codeHash?: string
}

export const appIdentity = (host: LaunchHost, appId: string): string => `${host.identity}.${appId}`

export const appIconPath = (host: LaunchHost, appId: string, platform: NodeJS.Platform = process.platform): string =>
  join(host.iconDir, `${appId}.${platform === 'win32' ? 'ico' : 'png'}`)

export function appLaunchArg(argv: readonly string[]): string | null {
  const id = argv.find((arg) => arg.startsWith(APP_ARG))?.slice(APP_ARG.length)
  return id && VALID_APP_ID_RE.test(id) ? id : null
}

export function windowsArgs(host: LaunchHost, appId: string): string {
  return [...host.args, APP_ARG + appId].map((arg) => (/[\s"]/.test(arg) ? `"${arg.replaceAll('"', '\\"')}"` : arg)).join(' ')
}

export function launcherName(title: string, appId: string): string {
  const cleaned = title
    .replace(/[<>:"/\\|?*\p{Cc}]/gu, ' ')
    .replace(/\s+/g, ' ')
    .trim()
    .replace(/^\.+/, '')
    .slice(0, 60)
    .trim()
  return cleaned || `Jaz App ${appId.slice(0, 8)}`
}

export function createLauncher(
  spec: LauncherSpec,
  host: LaunchHost,
  platform: NodeJS.Platform = process.platform,
  home = homedir(),
  env: NodeJS.ProcessEnv = process.env,
): Launcher {
  mkdirSync(host.iconDir, { recursive: true })
  writeFileSync(join(host.iconDir, `${spec.appId}.png`), spec.png(1024))
  if (platform === 'darwin') return createShimBundle(spec, host, join(home, 'Applications', host.folder))
  if (platform === 'win32') {
    const dir = join(env['APPDATA'] ?? join(home, 'AppData', 'Roaming'), 'Microsoft', 'Windows', 'Start Menu', 'Programs', host.folder)
    mkdirSync(dir, { recursive: true })
    const icon = appIconPath(host, spec.appId, platform)
    writeFileSync(icon, ico(spec.png(256)))
    const launcherPath = freePath(dir, spec, '.lnk')
    const written = host.writeShortcut(launcherPath, {
      target: host.executable,
      args: windowsArgs(host, spec.appId),
      description: spec.title,
      icon,
      iconIndex: 0,
      appUserModelId: appIdentity(host, spec.appId),
    })
    if (!written) throw new Error('Could not add the app to the Start menu')
    return { launcherPath }
  }
  if (platform === 'linux') {
    const dir = join(env['XDG_DATA_HOME'] || join(home, '.local', 'share'), 'applications')
    mkdirSync(dir, { recursive: true })
    const launcherPath = join(dir, `${appIdentity(host, spec.appId)}.desktop`)
    const exec = [host.executable, ...host.args, APP_ARG + spec.appId].map((arg) => `"${arg.replace(/["`$\\]/g, '\\$&')}"`).join(' ')
    writeFileSync(launcherPath, [
      '[Desktop Entry]',
      'Type=Application',
      `Name=${launcherName(spec.title, spec.appId)}`,
      `Exec=${exec}`,
      `Icon=${appIconPath(host, spec.appId, platform)}`,
      `StartupWMClass=${appIdentity(host, spec.appId)}`,
      'Terminal=false',
      'Categories=Utility;',
      '',
    ].join('\n'))
    return { launcherPath }
  }
  return {}
}

export function removeLauncher(host: LaunchHost, appId: string, launcherPath?: string): void {
  for (const path of [launcherPath, join(host.iconDir, `${appId}.png`), join(host.iconDir, `${appId}.ico`)]) {
    if (path) rmSync(path, { recursive: true, force: true })
  }
}

// The app a macOS launcher opens, as its bundle records it.
export function shimAppId(bundlePath: string): string | undefined {
  try {
    return plistString(readFileSync(join(bundlePath, 'Contents', 'Info.plist'), 'utf8'), 'JazAppId')
  } catch {
    return undefined
  }
}

function createShimBundle(spec: LauncherSpec, host: LaunchHost, dir: string): Launcher {
  const hostBundle = resolve(host.executable, '../../..')
  const hostInfo = readFileSync(join(hostBundle, 'Contents', 'Info.plist'), 'utf8')
  const name = launcherName(spec.title, spec.appId)
  const staging = join(dir, `.${spec.appId}.installing`)
  rmSync(staging, { recursive: true, force: true })
  mkdirSync(join(staging, 'Contents', 'MacOS'), { recursive: true })
  mkdirSync(join(staging, 'Contents', 'Resources'))
  writeFileSync(join(staging, 'Contents', 'Info.plist'), plist({
    CFBundleIdentifier: appIdentity(host, spec.appId),
    CFBundleName: name,
    CFBundleDisplayName: name,
    CFBundleExecutable: 'app_shim',
    CFBundleIconFile: 'app',
    CFBundlePackageType: 'APPL',
    CFBundleShortVersionString: '1.0',
    CFBundleVersion: '1',
    LSMinimumSystemVersion: plistString(hostInfo, 'LSMinimumSystemVersion') ?? '12.0',
    NSHighResolutionCapable: true,
    NSPrincipalClass: 'NSApplication',
    JazAppId: spec.appId,
    ElectronAppShimHostPath: hostBundle,
    ElectronAppShimHostBundleIdentifier: plistString(hostInfo, 'CFBundleIdentifier') ?? '',
    ElectronAppShimServerName: host.serverName,
    ElectronAppShimHostArguments: [...host.args, SHIM_LAUNCH_ARG],
  }))
  writeFileSync(join(staging, 'Contents', 'PkgInfo'), 'APPL????')
  // A fresh copy of the signed loader, so it keeps the signature Jaz's framework
  // requires but none of the extended attributes of the file it came from.
  const loader = join(hostBundle, 'Contents', 'Frameworks', 'Electron Framework.framework', 'Helpers', 'app_shim_loader')
  writeFileSync(join(staging, 'Contents', 'MacOS', 'app_shim'), readFileSync(loader), { mode: 0o755 })
  writeFileSync(join(staging, 'Contents', 'Resources', 'app.icns'), icns(spec.png))
  const launcherPath = freePath(dir, spec, '.app')
  renameSync(staging, launcherPath)
  return { launcherPath, codeHash: cdhash(join(launcherPath, 'Contents', 'MacOS', 'app_shim')) }
}

function freePath(dir: string, spec: LauncherSpec, extension: string): string {
  const name = launcherName(spec.title, spec.appId)
  const path = join(dir, name + extension)
  return existsSync(path) ? join(dir, `${name} ${spec.appId.slice(0, 6)}${extension}`) : path
}

function cdhash(path: string): string {
  const result = spawnSync('/usr/bin/codesign', ['-dvvv', path], { encoding: 'utf8' })
  const hash = /^CDHash=([0-9a-f]{40})$/m.exec(result.stderr ?? '')?.[1]
  if (!hash) throw new Error('Could not read the app launcher signature')
  return hash
}

function plistString(text: string, key: string): string | undefined {
  return new RegExp(`<key>${key}</key>\\s*<string>([^<]*)</string>`).exec(text)?.[1]
}

function escapeXml(value: string): string {
  return value.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;')
}

function plist(entries: Record<string, string | boolean | string[]>): string {
  const string = (value: string): string => `<string>${escapeXml(value)}</string>`
  const body = Object.entries(entries).map(([key, value]) =>
    `  <key>${key}</key>\n  ${typeof value === 'boolean' ? `<${value}/>` : Array.isArray(value) ? `<array>${value.map(string).join('')}</array>` : string(value)}`)
  return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
${body.join('\n')}
</dict>
</plist>
`
}

// An icns file is a chunk of PNG chunks, each a four-letter type and a big-endian length.
function icns(png: (size: number) => Buffer): Buffer {
  const chunk = (type: string, data: Buffer): Buffer => {
    const head = Buffer.alloc(8)
    head.write(type, 'latin1')
    head.writeUInt32BE(data.length + 8, 4)
    return Buffer.concat([head, data])
  }
  const sizes = [['ic07', 128], ['ic08', 256], ['ic09', 512], ['ic10', 1024]] as const
  return chunk('icns', Buffer.concat(sizes.map(([type, size]) => chunk(type, png(size)))))
}

// A single 256 px PNG image in an ico container, which Windows shortcuts and taskbars need.
function ico(png: Buffer): Buffer {
  const head = Buffer.alloc(22)
  head.writeUInt16LE(1, 2)
  head.writeUInt16LE(1, 4)
  head.writeUInt16LE(1, 10)
  head.writeUInt16LE(32, 12)
  head.writeUInt32LE(png.length, 14)
  head.writeUInt32LE(22, 18)
  return Buffer.concat([head, png])
}
