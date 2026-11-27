export function cloudWorkspaceHeaders(workspaceID?: string): Record<string, string> {
  return workspaceID ? { 'X-Jaz-Cloud-Workspace': workspaceID } : {}
}
