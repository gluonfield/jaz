import type { CloudWorkspaceSummary } from '@/lib/cloudAccount'

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

export function workspaceSubtitle(workspace: Pick<CloudWorkspaceSummary, 'org_name' | 'email' | 'url'>): string {
  const host = urlHost(workspace.url)
  const email = workspace.email?.trim()
  if (email && workspace.org_name?.trim()) return email
  if (email && host && email !== host) return `${email} · ${host}`
  return email || host
}

export function workspaceInitials(name: string): string {
  const parts = name
    .trim()
    .split(/[\s@._-]+/)
    .filter(Boolean)
  if (parts.length === 0) return 'C'
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase()
  return `${parts[0][0] ?? ''}${parts[1][0] ?? ''}`.toUpperCase()
}
