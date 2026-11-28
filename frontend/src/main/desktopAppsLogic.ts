import { existsSync, realpathSync } from 'node:fs'
import { dirname, isAbsolute, join, relative, resolve } from 'node:path'
import {
  declaredDesktopPermissions,
  isMachineAccess,
  type DesktopAppPermission,
  type InstalledDesktopApp,
} from '../shared/desktopApps'

export const VALID_APP_ID_RE = /^[a-z0-9]{26}$/

// The permissions an install may grant: only what the app declares, never machine
// access for apps from outside the org, and full access only where the org allows it.
export function filterPermissionsForPolicy(
  requested: readonly DesktopAppPermission[],
  declaredCapabilities: readonly string[],
  isExternalOrg: boolean,
  allowDesktopFullAccess: boolean,
): DesktopAppPermission[] {
  return declaredDesktopPermissions(declaredCapabilities).filter((permission) =>
    requested.includes(permission) &&
    !(isExternalOrg && isMachineAccess(permission)) &&
    (permission !== 'full_access' || allowDesktopFullAccess))
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
  if (record.isExternalOrg && isMachineAccess(permission)) return false
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
