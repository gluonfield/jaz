import type { CloudAccountState, CloudWorkspaceSummary } from '@/lib/cloudAccount'

export const LOCAL_WORKSPACE_ID = 'local'

function urlHost(raw: string): string {
  try {
    return new URL(raw).host
  } catch {
    return raw
  }
}

export function workspaceDisplayName(workspace: Pick<CloudWorkspaceSummary, 'org_name' | 'url'>): string {
  const org = workspace.org_name?.trim()
  return org || urlHost(workspace.url)
}

type WorkspaceSelection = Pick<CloudAccountState, 'activeWorkspaceId' | 'workspaces'>

export function selectedCloudWorkspace(cloud: WorkspaceSelection): CloudWorkspaceSummary | null {
  return cloud.workspaces.find((ws) => ws.id === cloud.activeWorkspaceId) ?? null
}

export function activeCloudWorkspace(cloud: WorkspaceSelection & { active: boolean }): CloudWorkspaceSummary | null {
  return cloud.active ? selectedCloudWorkspace(cloud) : null
}
