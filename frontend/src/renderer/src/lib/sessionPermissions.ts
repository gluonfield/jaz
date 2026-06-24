import type { ACPPermission, SessionEvent } from '@/lib/api/types'

function normalized(value: string | undefined): string {
  return (value ?? '').trim().toLowerCase()
}

export function hasPermissionSurface(permission: ACPPermission | undefined): boolean {
  if (!permission?.id?.trim()) return false
  return Boolean(
    permission.questions?.length ||
      permission.options?.length ||
      permission.locations?.length,
  )
}

export function isPermissionAwaitingResponse(permission: ACPPermission | undefined): boolean {
  if (!hasPermissionSurface(permission)) return false
  const status = normalized(permission?.status)
  return status !== 'selected' && status !== 'cancelled'
}

export function activePermissionIDs(events: SessionEvent[], permissions: ACPPermission[] = []): Set<string> {
  const active = new Set<string>()
  for (const permission of permissions) {
    if (isPermissionAwaitingResponse(permission)) {
      active.add(permission.id)
    }
  }
  for (const event of events) {
    const id = event.permission?.id
    if (!id) continue
    if (event.type === 'permission_request' && isPermissionAwaitingResponse(event.permission)) {
      active.add(id)
    }
    if (event.type === 'permission_response') {
      active.delete(id)
    }
  }
  return active
}

export function resolveInactivePermissions(events: SessionEvent[], active: Set<string>): SessionEvent[] {
  const resolved = new Set<string>()
  for (const event of events) {
    if (event.type === 'permission_response' && event.permission?.id) {
      resolved.add(event.permission.id)
    }
  }
  return events.flatMap((event) => {
    const permission = event.permission
    if (
      event.type !== 'permission_request' ||
      !permission?.id ||
      active.has(permission.id) ||
      resolved.has(permission.id) ||
      !isPermissionAwaitingResponse(permission)
    ) {
      return [event]
    }
    resolved.add(permission.id)
    return [
      event,
      {
        ...event,
        type: 'permission_response',
        permission: { ...permission, status: 'cancelled' },
      },
    ]
  })
}
