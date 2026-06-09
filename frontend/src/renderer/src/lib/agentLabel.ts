// Prettifies an ACP agent name for display, e.g. "claude" → "Claude".
export function agentLabel(value: string | undefined): string {
  const normalizedAgent = (value || 'agent').trim()
  if (!normalizedAgent) return 'Agent'
  if (normalizedAgent.toLowerCase() === 'codex') return 'Codex'
  return normalizedAgent
    .split(/[_\s-]+/)
    .filter(Boolean)
    .map((part) => part.slice(0, 1).toUpperCase() + part.slice(1))
    .join(' ')
}
