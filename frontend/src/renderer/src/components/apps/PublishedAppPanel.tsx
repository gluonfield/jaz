import { useQuery } from '@tanstack/react-query'
import { AppRenderer } from '@/components/apps/AppRenderer'
import { EmptyState } from '@/components/ui/EmptyState'
import { appDetailQuery } from '@/lib/api/apps'

// A published app running in a thread's side panel, beside the chat.
export function PublishedAppPanel({ appId }: { appId: string }) {
  const detail = useQuery(appDetailQuery(appId))
  if (detail.isPending) return null
  if (!detail.data) return <EmptyState title="App unavailable">It may have been deleted, or you no longer have access.</EmptyState>
  const { app, current_version } = detail.data
  return <AppRenderer app={{ ...app, bundle_sha256: app.bundle_sha256 ?? current_version?.bundle_sha256 }} />
}
