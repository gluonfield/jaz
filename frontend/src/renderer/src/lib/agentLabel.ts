// Curated display names for known agents; the slug ("claude") is the
// backend identifier, not what we want to show. Everything else falls back to
// title-casing the slug ("local_helper" → "Local Helper").
const DISPLAY_NAMES: Record<string, string> = {
  codex: 'Codex',
  claude: 'Claude Code',
  grok: 'Grok',
}

// Prettifies an ACP agent name for display, e.g. "claude" → "Claude Code".
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

// The identity provider you actually authenticate against — the OAuth is with
// the model maker, not the CLI. "claude" (Claude Code) signs in with Anthropic,
// "codex" with OpenAI, "grok" with xAI. Used for "Sign in with …" copy.
const AUTH_PROVIDERS: Record<string, string> = {
  codex: 'OpenAI',
  claude: 'Anthropic',
  grok: 'xAI',
}

export function authProviderLabel(value: string | undefined): string {
  const slug = (value || '').trim().toLowerCase().replace(/[\s-]+/g, '_')
  return AUTH_PROVIDERS[slug] ?? agentLabel(value)
}
