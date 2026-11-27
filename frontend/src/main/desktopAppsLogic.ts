import { chmodSync, existsSync, mkdirSync, realpathSync, rmSync, writeFileSync } from 'node:fs'
import { homedir } from 'node:os'
import { dirname, isAbsolute, join, relative, resolve } from 'node:path'
import {
  isTier3Permission,
  type DesktopAppPermission,
  type InstalledDesktopApp,
} from '../shared/desktopApps'

export const VALID_APP_ID_RE = /^[a-z0-9]{26}$/

export function parseDesktopAppDeepLink(rawUrl: string): string | null {
  try {
    const parsed = new URL(rawUrl.trim())
    if (parsed.protocol !== 'jaz:' || parsed.hostname !== 'app') return null
    const id = parsed.pathname.replace(/^\/+|\/+$/g, '')
    return VALID_APP_ID_RE.test(id) ? id : null
  } catch {
    return null
  }
}

export function extractDesktopAppDeepLinkArg(argv: readonly string[]): string | null {
  for (const arg of argv) {
    if (typeof arg !== 'string') continue
    const id = parseDesktopAppDeepLink(arg)
    if (id) return id
  }
  return null
}

export function sanitizeLauncherName(title: string, appId: string): string {
  const cleaned = title
    .replace(/[<>:"/\\|?*\x00-\x1f]/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
    .slice(0, 60)
  return cleaned || `Jaz App ${appId.slice(0, 8)}`
}

export function filterPermissionsForPolicy(
  requested: readonly DesktopAppPermission[],
  isExternalOrg: boolean,
  allowDesktopFullAccess: boolean,
): DesktopAppPermission[] {
  const seen = new Set<DesktopAppPermission>()
  const out: DesktopAppPermission[] = []
  for (const perm of requested) {
    if (isExternalOrg && isTier3Permission(perm)) continue
    if (perm === 'full_access' && !allowDesktopFullAccess) continue
    if (!seen.has(perm)) {
      seen.add(perm)
      out.push(perm)
    }
  }
  return out
}

export function syncDesktopAppVersionRecord(
  record: InstalledDesktopApp,
  version: number,
  latestBundleSha256: string,
  declaredCapabilities?: string[],
): InstalledDesktopApp {
  const nextSha = latestBundleSha256.trim() || record.latestBundleSha256
  const changed = Boolean(nextSha && record.approvedBundleSha256 && nextSha !== record.approvedBundleSha256)
  return {
    ...record,
    installedVersion: version > 0 ? version : record.installedVersion,
    latestBundleSha256: nextSha,
    declaredCapabilities: declaredCapabilities ?? record.declaredCapabilities,
    permissionsPaused: record.permissionsPaused || (changed && record.permissions.length > 0),
    updatedAt: new Date().toISOString(),
  }
}

export function canUseDesktopPermission(
  record: InstalledDesktopApp | undefined,
  permission: DesktopAppPermission,
): boolean {
  if (!record) return false
  if (record.permissionsPaused) return false
  if (!record.approvedBundleSha256 || record.latestBundleSha256 !== record.approvedBundleSha256) return false
  if (record.isExternalOrg && isTier3Permission(permission)) return false
  return record.permissions.includes(permission)
}

export function resolveScopedFolderTarget(
  record: InstalledDesktopApp | undefined,
  folderPath: string,
  relPath: string,
): string {
  if (!canUseDesktopPermission(record, 'fs')) {
    throw new Error('Scoped folder permission is not approved for this app version')
  }
  const normalizedFolder = resolve(folderPath)
  const allowed = (record!.scopedFolders ?? []).some((f) => resolve(f) === normalizedFolder)
  if (!allowed) {
    throw new Error('Folder is not in the approved scopedFolders list for this app')
  }
  if (relPath.includes('\0') || isAbsolute(relPath)) {
    throw new Error('Path must stay inside the approved folder')
  }
  const realFolder = realpathSync(normalizedFolder)
  const candidate = resolve(realFolder, relPath || '.')
  const rel = relative(realFolder, candidate)
  if (rel.startsWith('..') || isAbsolute(rel)) {
    throw new Error('Path escapes approved folder')
  }
  // Walk up to the nearest existing ancestor so creating a new file under a
  // symlinked subdirectory cannot escape the approved folder.
  let existingAncestor = candidate
  const tailSegments: string[] = []
  while (!existsSync(existingAncestor)) {
    const parent = dirname(existingAncestor)
    if (parent === existingAncestor) break
    tailSegments.unshift(relative(parent, existingAncestor))
    existingAncestor = parent
  }
  const realAncestor = realpathSync(existingAncestor)
  const ancestorRel = relative(realFolder, realAncestor)
  if (ancestorRel.startsWith('..') || isAbsolute(ancestorRel)) {
    throw new Error('Symlink escapes approved folder')
  }
  return tailSegments.length > 0 ? join(realAncestor, ...tailSegments) : realAncestor
}

function escapeXml(value: string): string {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&apos;')
}

export function createOSLauncherShim(
  title: string,
  appId: string,
  platform: NodeJS.Platform = process.platform,
  homeDir = homedir(),
  appDataDir = process.env['APPDATA'] ?? join(homeDir, 'AppData', 'Roaming'),
): string | undefined {
  const safeName = sanitizeLauncherName(title, appId)
  const deepLink = `jaz://app/${appId}`

  if (platform === 'darwin') {
    const appsDir = join(homeDir, 'Applications', 'Jaz Apps')
    const bundleDir = join(appsDir, `${safeName}.app`)
    const contentsDir = join(bundleDir, 'Contents')
    const macosDir = join(contentsDir, 'MacOS')
    mkdirSync(macosDir, { recursive: true })
    const plist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key>
  <string>${escapeXml(safeName)}</string>
  <key>CFBundleDisplayName</key>
  <string>${escapeXml(safeName)}</string>
  <key>CFBundleIdentifier</key>
  <string>dev.wins.jaz.app.${appId}</string>
  <key>CFBundleExecutable</key>
  <string>launch</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleVersion</key>
  <string>1.0</string>
  <key>LSUIElement</key>
  <true/>
</dict>
</plist>
`
    writeFileSync(join(contentsDir, 'Info.plist'), plist, 'utf8')
    const execPath = join(macosDir, 'launch')
    writeFileSync(execPath, `#!/bin/sh\nexec open "${deepLink}"\n`, 'utf8')
    chmodSync(execPath, 0o755)
    return bundleDir
  }

  if (platform === 'win32') {
    const startDir = join(appDataDir, 'Microsoft', 'Windows', 'Start Menu', 'Programs', 'Jaz Apps')
    mkdirSync(startDir, { recursive: true })
    const urlFile = join(startDir, `${safeName}.url`)
    writeFileSync(urlFile, `[InternetShortcut]\r\nURL=${deepLink}\r\n`, 'utf8')
    return urlFile
  }

  return undefined
}

export function removeOSLauncherShim(launcherPath?: string): void {
  if (!launcherPath) return
  try {
    rmSync(launcherPath, { recursive: true, force: true })
  } catch {
    // best-effort cleanup
  }
}
