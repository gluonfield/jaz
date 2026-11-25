import type { CloudWorkspaceSummary } from '@/lib/cloudAccount'

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

export function activeCloudWorkspace(
  cloud: { active: boolean; activeWorkspaceId: string; workspaces: CloudWorkspaceSummary[] },
): CloudWorkspaceSummary | null {
  if (!cloud.active || cloud.activeWorkspaceId === LOCAL_WORKSPACE_ID) return null
  return cloud.workspaces.find((ws) => ws.id === cloud.activeWorkspaceId) ?? cloud.workspaces[0] ?? null
}
