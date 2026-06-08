import type { ACPPermission } from '@/lib/api/types'

export function normalized(value: string | undefined): string {
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
