import type { McpUiDisplayMode, McpUiHostContext, McpUiStyleVariableKey, McpUiStyles } from '@modelcontextprotocol/ext-apps/app-bridge'

// MCP Apps theme through the spec's standard style keys; each is fed by the
// Jaz token playing the same role, resolved to a concrete value.
const STYLE_TOKENS: [McpUiStyleVariableKey, string][] = [
  ['--color-background-primary', '--color-bg'],
  ['--color-background-secondary', '--color-panel'],
  ['--color-background-tertiary', '--color-surface'],
  ['--color-text-primary', '--color-ink'],
  ['--color-text-secondary', '--color-ink-2'],
  ['--color-text-tertiary', '--color-ink-3'],
  ['--color-text-danger', '--color-danger'],
  ['--color-text-success', '--color-ok'],
  ['--color-text-warning', '--color-running'],
  ['--color-border-primary', '--color-border'],
  ['--color-ring-primary', '--color-primary'],
  ['--font-sans', '--font-sans'],
  ['--font-mono', '--font-mono'],
  ['--border-radius-md', '--radius-control'],
  ['--border-radius-lg', '--radius-card'],
  ['--shadow-lg', '--shadow-raised'],
]

// An inline view sets its own height; any other fills its frame.
export function mcpAppHostContext(displayMode: 'inline' | 'fullscreen', availableDisplayModes: McpUiDisplayMode[], frame: HTMLIFrameElement): McpUiHostContext {
  const root = document.documentElement
  const style = getComputedStyle(root)
  // McpUiStyles names every key; hosts send the subset they have.
  const variables = Object.fromEntries(
    STYLE_TOKENS.map(([key, token]) => [key, style.getPropertyValue(token).trim()]),
  ) as McpUiStyles
  return {
    theme: root.classList.contains('dark') ? 'dark' : 'light',
    styles: { variables },
    displayMode,
    availableDisplayModes,
    containerDimensions: { width: frame.clientWidth, ...(displayMode === 'fullscreen' && { height: frame.clientHeight }) },
    platform: 'desktop',
    locale: navigator.language,
  }
}
