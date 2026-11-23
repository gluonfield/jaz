export type WorkspaceSwitchRoute = '/new' | '/bots' | '/boards' | '/loops'

export interface WorkspaceSwitchNavigation {
  to: WorkspaceSwitchRoute
  search?: Record<string, unknown>
  replace?: boolean
}

export function workspaceSwitchDestination(pathname: string): WorkspaceSwitchRoute | null {
  if (pathname.startsWith('/sessions/')) return '/new'
  if (pathname.startsWith('/bots/') && pathname !== '/bots/') return '/bots'
  if (pathname.startsWith('/boards/') && pathname !== '/boards/') return '/boards'
  if (pathname.startsWith('/loops/') && pathname !== '/loops/') return '/loops'
  if (pathname.startsWith('/plugins/')) return '/new'
  if (pathname.startsWith('/apps/')) return '/new'
  return null
}

export function workspaceSwitchNavigation(location: {
  pathname: string
  search?: Record<string, unknown>
}): WorkspaceSwitchNavigation | null {
  const settings = location.search?.settings
  const target = workspaceSwitchDestination(location.pathname)
  if (target) {
    return settings !== undefined
      ? { to: target, search: { settings }, replace: true }
      : { to: target }
  }
  if (location.pathname === '/new' && location.search?.space) {
    return {
      to: '/new',
      search: settings !== undefined ? { settings } : {},
      replace: true,
    }
  }
  return null
}
