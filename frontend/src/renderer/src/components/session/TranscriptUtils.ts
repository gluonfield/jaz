export { hasPermissionSurface, isPlanApprovalPermission } from '@/lib/sessionPermissions'

export function normalized(value: string | undefined): string {
  return (value ?? '').trim().toLowerCase()
}
