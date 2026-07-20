// Curated display names for known agents; the slug ("claude") is the
// backend identifier, not what we want to show. Everything else falls back to
// title-casing the slug ("local_helper" → "Local Helper").
const DISPLAY_NAMES: Record<string, string> = {
  jaz: 'Jaz',
  codex: 'Codex',
  claude: 'Claude',
  kimi: 'Kimi',
  qwen: 'Qwen',
  grok: 'Grok',
  opencode: 'OpenCode',
  antigravity: 'Antigravity',
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

// The identity provider you actually authenticate against — the OAuth is with
// the model maker, not the CLI. "claude" (Claude Code) signs in with Anthropic,
// "codex" with OpenAI, "kimi" with Moonshot AI, "grok" with xAI. Used for
// "Sign in with …" copy.
const AUTH_PROVIDERS: Record<string, string> = {
  codex: 'OpenAI',
  claude: 'Anthropic',
  kimi: 'Moonshot AI',
  qwen: 'Qwen Coding Plan',
  grok: 'xAI',
  opencode: 'OpenRouter',
  antigravity: 'Google AI',
}

export function authProviderLabel(value: string | undefined): string {
  const slug = (value || '').trim().toLowerCase().replace(/[\s-]+/g, '_')
  return AUTH_PROVIDERS[slug] ?? agentLabel(value)
}

// Onboarding spells Claude out as "Claude Code" so first-time users recognise
// the CLI they're connecting; everywhere else (sidebar, runtime badges) the
// shorter "Claude" reads better. Only the onboarding screen uses this variant.
const ONBOARDING_NAMES: Record<string, string> = {
  claude: 'Claude Code',
  kimi: 'Kimi Code',
  qwen: 'Qwen Code',
  antigravity: 'Antigravity',
}

export function onboardingAgentLabel(value: string | undefined): string {
  const slug = (value || '').trim().toLowerCase().replace(/[\s-]+/g, '_')
  return ONBOARDING_NAMES[slug] ?? agentLabel(value)
}

export function agentAPIKeyCopy(
  value: string | undefined,
  target: string,
  configured: boolean,
): { placeholder: string; description: string; connected: string } {
  if ((value || '').trim().toLowerCase() === 'qwen') {
    return {
      placeholder: configured ? 'Already set up' : 'Paste your sk-sp-… subscription key',
      description: 'Uses your Alibaba Cloud Coding Plan subscription; Qwen OAuth is discontinued.',
      connected: 'Connected to Qwen Coding Plan',
    }
  }
  return {
    placeholder: configured ? 'Already set up' : 'Paste an API key',
    description: `jaz passes this key straight to ${target}.`,
    connected: 'Connected with an API key',
  }
}
