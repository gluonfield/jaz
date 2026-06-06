export const keys = {
  health: ['health'] as const,
  rootSessions: ['sessions', 'root'] as const,
  allSessions: ['sessions', 'all'] as const,
  sessionMessages: (id: string) => ['sessions', id, 'messages'] as const,
  sessionEvents: (id: string) => ['sessions', id, 'events'] as const,
  agentFiles: ['agent', 'files'] as const,
}
