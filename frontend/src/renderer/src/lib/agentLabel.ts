// Curated display names for known agents; the slug ("claude") is the
// backend identifier, not what we want to show. Everything else falls back to
// title-casing the slug ("local_helper" → "Local Helper").
const DISPLAY_NAMES: Record<string, string> = {
  codex: 'Codex',
  claude: 'Claude',
}

// Prettifies an ACP agent name for display, e.g. "claude" → "Claude".
export function agentLabel(value: string | undefined): string {
  const slug = (value || '').trim()
  if (!slug) return 'Agent'
  const known = DISPLAY_NAMES[slug.toLowerCase().replace(/[\s-]+/g, '_')]
  if (known) return known
  return slug
    .split(/[_\s-]+/)
    .filter(Boolean)
    .map((part) => part.slice(0, 1).toUpperCase() + part.slice(1))
    .join(' ')
}
