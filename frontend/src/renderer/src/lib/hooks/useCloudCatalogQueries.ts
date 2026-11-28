import { cloudMachinesQuery, cloudModelProfilesQuery } from '@/lib/api/cloudThreads'
import { useCloudCatalogScope } from '@/lib/cloudSession'

export function useCloudModelProfilesQuery() {
  const { account, workspaceID } = useCloudCatalogScope()
  return cloudModelProfilesQuery(account, workspaceID)
}

export function useCloudMachinesQuery() {
  const { account, workspaceID } = useCloudCatalogScope()
  return cloudMachinesQuery(account, workspaceID)
}
