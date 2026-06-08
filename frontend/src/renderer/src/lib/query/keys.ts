export const keys = {
  health: ['health'] as const,
  sidebarSessions: ['sessions', 'sidebar'] as const,
  allSessions: ['sessions', 'all'] as const,
  archivedSessions: ['sessions', 'archived'] as const,
  sessionMessages: (id: string) => ['sessions', id, 'messages'] as const,
  sessionEvents: (id: string) => ['sessions', id, 'events'] as const,
  agentFiles: ['agent', 'files'] as const,
  mcpServers: ['mcp', 'servers'] as const,
}
