export const keys = {
  health: ['health'] as const,
  sidebarSessions: ['sessions', 'sidebar'] as const,
  allSessions: ['sessions', 'all'] as const,
  session: (id: string) => ['sessions', id] as const,
  usage: ['usage'] as const,
  usageDaily: (days: number, timezone: string) => ['usage', 'daily', days, timezone] as const,
  archivedSessions: ['sessions', 'archived'] as const,
  threadSearch: (query: string, includeArchived = false) =>
    ['search', 'threads', query, includeArchived] as const,
  sessionMessages: (id: string) => ['sessions', id, 'messages'] as const,
  sessionRepo: (id: string) => ['sessions', id, 'repo'] as const,
  // Children of sessionRepo so one prefix invalidation refreshes repo state,
  // the changes summary, and any cached file diffs together.
  sessionRepoChanges: (id: string) => ['sessions', id, 'repo', 'changes'] as const,
  // The key carries the full request identity (base, rename source) so a
  // moved base creates a fresh entry instead of silently reusing a patch
  // pinned to the old one.
  sessionRepoDiff: (id: string, fileKey: string, base: string, oldPath: string) =>
    ['sessions', id, 'repo', 'diff', fileKey, base, oldPath] as const,
  sessionFile: (id: string, path: string) => ['sessions', id, 'file', path] as const,
  sessionEvents: (id: string) => ['sessions', id, 'events'] as const,
  agentFiles: ['agent', 'files'] as const,
  agentSettings: ['settings', 'agents'] as const,
  devices: ['settings', 'devices'] as const,
  deviceConnectionLink: ['settings', 'devices', 'connection-link'] as const,
  onboarding: ['onboarding'] as const,
  memory: ['memory'] as const,
  connectionPlugins: ['connections', 'plugins'] as const,
  connectionQR: (id: string) => ['connections', 'qr', id] as const,
  browserSettings: ['browser', 'settings'] as const,
  mcpServers: ['mcp', 'servers'] as const,
  acpAgents: ['acp', 'agents'] as const,
  openRouterModels: ['openrouter', 'models'] as const,
  projects: ['projects'] as const,
  filesystemDirs: (path: string) => ['filesystem', 'dirs', path] as const,
  workspaceFiles: (root: string) => ['workspace', 'files', root] as const,
  skills: (root?: string) => ['skills', root ?? null] as const,
  loops: ['loops'] as const,
  loopDetail: (id: string) => ['loops', id] as const,
  boards: ['boards'] as const,
  boardDetail: (id: string) => ['boards', id] as const,
}
