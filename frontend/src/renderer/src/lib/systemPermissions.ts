import { useMutation, useQuery } from '@tanstack/react-query'
import type { SystemPermission } from '@shared/systemPermissions'
import { keys } from '@/lib/query/keys'

// Privacy permissions exist only in the macOS desktop app.
export const hasSystemPermissions = Boolean(window.jaz) && /Mac/i.test(navigator.platform)

// macOS changes these outside Jaz, so the status is polled while shown.
export function useSystemPermissions(enabled = true) {
  const status = useQuery({
    queryKey: keys.systemPermissions,
    queryFn: () => window.jaz!.systemPermissions.status(),
    enabled: hasSystemPermissions && enabled,
    refetchInterval: 1500,
  })
  const allow = useMutation({
    mutationFn: (permission: SystemPermission) => window.jaz!.systemPermissions.allow(permission),
    onSettled: () => status.refetch(),
  })
  return { status: status.data, allow }
}
