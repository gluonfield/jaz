import { hasBundle, type BundledApp, type JazApp } from '@/lib/api/apps'
import type { InstalledDesktopApp } from '@shared/desktopApps'

export const APPS_TABS = ['mine', 'team', 'desktop', 'connected', 'boards'] as const
export type AppsTab = (typeof APPS_TABS)[number]

// An installed app whose published record is gone still shows, from what the
// desktop kept at install time.
function installedAsApp(installed: InstalledDesktopApp): BundledApp {
  return {
    id: installed.appId,
    org_id: installed.workspaceId !== 'local' ? installed.workspaceId : undefined,
    owner_user_id: 'local',
    title: installed.title,
    description: installed.description,
    icon: installed.icon,
    kind: installed.kind,
    visibility: 'private',
    current_version: installed.installedVersion,
    status: 'active',
    bundle_sha256: installed.latestBundleSha256,
    capabilities: installed.declaredCapabilities,
    created_at: installed.installedAt,
    updated_at: installed.updatedAt,
  }
}

// What a library tab lists. Boards have a tab of their own and appear on no
// other; connected apps come from MCP servers, not from here.
export function libraryApps(tab: AppsTab, published: JazApp[], installed: InstalledDesktopApp[]): JazApp[] {
  if (tab === 'boards') return published.filter((app) => !hasBundle(app))
  const apps = published.filter(hasBundle)
  if (tab === 'mine') return apps.filter((app) => app.is_owner !== false)
  if (tab === 'team') return apps.filter((app) => app.visibility !== 'private')
  if (tab === 'desktop') {
    return installed.map((record) => apps.find((app) => app.id === record.appId) ?? installedAsApp(record))
  }
  return []
}
