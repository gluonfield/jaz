import type { BundledApp, JazApp } from '@/lib/api/apps'
import { clientRuntime } from '@/lib/clientRuntime'
import type { DesktopAppPermission, InstalledDesktopApp } from '@shared/desktopApps'

export const desktopApps = clientRuntime.kind === 'electron' ? clientRuntime.desktopApps : undefined

// Installs an app as a program of its own and opens it.
export async function installDesktopApp(
  app: BundledApp,
  permissions: DesktopAppPermission[] = [],
  scopedFolders: string[] = [],
  allowDesktopFullAccess = false,
): Promise<InstalledDesktopApp> {
  const installed = await desktopApps!.install({
    appId: app.id,
    workspaceId: app.org_id || 'local',
    title: app.title,
    description: app.description,
    kind: app.kind,
    icon: app.icon,
    iconPng: drawDesktopAppIcon(app),
    version: app.current_version,
    bundleSha256: app.bundle_sha256 ?? '',
    declaredCapabilities: app.capabilities ?? [],
    permissions,
    scopedFolders,
    allowDesktopFullAccess,
  })
  await desktopApps!.open(app.id)
  return installed
}

// The folder an installed app's launcher is in, with the home folder shown as ~.
export function installFolder(launcherPath: string): string {
  return launcherPath
    .replace(/[\\/][^\\/]+$/, '')
    .replace(/^(\/Users\/[^/]+|\/home\/[^/]+|[A-Za-z]:\\Users\\[^\\]+)/, '~')
}

// The launcher icon: the app's emoji or initials on a rounded square, as a 1024 px PNG.
export function drawDesktopAppIcon(app: Pick<JazApp, 'id' | 'title' | 'icon'>): string {
  const canvas = document.createElement('canvas')
  canvas.width = 1024
  canvas.height = 1024
  const context = canvas.getContext('2d')!
  const hue = [...app.id].reduce((sum, char) => sum + char.charCodeAt(0), 0) % 360
  context.fillStyle = app.icon?.background || `hsl(${hue} 48% 44%)`
  context.beginPath()
  context.roundRect(100, 100, 824, 824, 185)
  context.fill()
  const value = app.icon?.value?.trim()
  const [text, size] = value && app.icon?.kind === 'emoji'
    ? [firstGrapheme(value), 500]
    : value && app.icon?.kind === 'initials'
      ? [value.slice(0, 2).toUpperCase(), 380]
      : [firstGrapheme(app.title.trim() || 'J').toUpperCase(), 460]
  context.fillStyle = app.icon?.color || '#ffffff'
  context.font = `600 ${size}px -apple-system, "Segoe UI", system-ui, sans-serif`
  context.textAlign = 'center'
  context.textBaseline = 'middle'
  context.fillText(text, 512, 530)
  return canvas.toDataURL('image/png')
}

function firstGrapheme(text: string): string {
  return new Intl.Segmenter().segment(text)[Symbol.iterator]().next().value?.segment ?? text
}
