import type { UsageWindow } from '@/lib/usageWindow'

export const keys = {
  health: ['health'] as const,
  sidebarSessions: ['sessions', 'sidebar'] as const,
  session: (id: string) => ['sessions', id] as const,
  usage: ['usage'] as const,
  usageDaily: (window: UsageWindow, timezone: string) => ['usage', 'daily', window, timezone] as const,
  archivedSessions: ['sessions', 'archived'] as const,
  feed: ['feed'] as const,
  threadSearch: (query: string, includeArchived = false) =>
    ['search', 'threads', query, includeArchived] as const,
  sessionMessages: (id: string) => ['sessions', id, 'messages'] as const,
  sessionOverview: (id: string) => ['sessions', id, 'overview'] as const,
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
  // Under agent settings: OpenAI sign-in and API-key changes decide voice availability.
  voiceSettings: ['settings', 'agents', 'voice'] as const,
  devices: ['settings', 'devices'] as const,
  deviceConnectionLink: ['settings', 'devices', 'connection-link'] as const,
  onboarding: ['onboarding'] as const,
  onboardingState: ['onboarding', 'state'] as const,
  memory: ['memory'] as const,
  connectionPlugins: ['connections', 'plugins'] as const,
  connectionQR: (id: string) => ['connections', 'qr', id] as const,
  computerSettings: ['computer', 'settings'] as const,
  computerStatus: ['computer', 'status'] as const,
  systemPermissions: ['system-permissions'] as const,
  browserSettings: ['browser', 'settings'] as const,
  mcp: ['mcp'] as const,
  mcpServers: ['mcp', 'servers'] as const,
  mcpApps: ['mcp', 'apps'] as const,
  mcpApp: (serverId: string, tool: string) => ['mcp', 'apps', serverId, tool] as const,
  acpAgents: ['acp', 'agents'] as const,
  openRouterModels: ['openrouter', 'models'] as const,
  providerStatuses: ['model-providers', 'status'] as const,
  modelProviderModels: (provider: string, agent: string) =>
    ['model-providers', provider, 'models', ...(agent ? [agent] : [])] as const,
  modelProviderStatus: (provider: string) => ['model-providers', provider, 'status'] as const,
  projects: ['projects'] as const,
  filesystemDirs: (path: string) => ['filesystem', 'dirs', path] as const,
  workspaceFiles: (root: string) => ['workspace', 'files', root] as const,
  skills: (root?: string) => ['skills', root ?? null] as const,
  loops: ['loops'] as const,
  loopDetail: (id: string) => ['loops', id] as const,
  // Under loops so a routine change refreshes every loop view with one prefix.
  botRoutines: (botId: string) => ['loops', 'bot', botId] as const,
  bots: ['bots'] as const,
  boards: ['boards'] as const,
  boardDetail: (id: string) => ['boards', id] as const,
}
