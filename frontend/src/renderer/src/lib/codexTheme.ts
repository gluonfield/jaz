import type { ColorScheme } from './appearanceScheme'

// Codex desktop share strings: reverse-engineered from ChatGPT.app / Codex
// Appearance → Copy theme. Format is `codex-theme-v1:` + JSON:
//   { codeThemeId, theme: { accent, contrast, fonts, ink, opaqueWindows,
//     semanticColors, surface }, variant: "light"|"dark" }
//
// Jaz maps the chrome colors only:
//   surface → background, ink → foreground, accent → accent, contrast → contrast.

export const CODEX_THEME_PREFIX = 'codex-theme-v1:'

export type CodexThemeVariant = 'light' | 'dark'

export interface CodexChromeTheme {
  accent: string
  contrast: number
  fonts: { code: string | null; ui: string | null }
  ink: string
  opaqueWindows: boolean
  semanticColors: {
    diffAdded: string
    diffRemoved: string
    skill: string
  }
  surface: string
}

export interface CodexThemeShare {
  codeThemeId: string
  theme: CodexChromeTheme
  variant: CodexThemeVariant
}

// Stock Codex chrome defaults (he.light / he.dark in the app asar).
export const CODEX_DEFAULT_CHROME: Record<CodexThemeVariant, CodexChromeTheme> = {
  light: {
    accent: '#339cff',
    contrast: 45,
    fonts: { code: null, ui: null },
    ink: '#1a1c1f',
    opaqueWindows: false,
    semanticColors: { diffAdded: '#00a240', diffRemoved: '#ba2623', skill: '#924ff7' },
    surface: '#ffffff',
  },
  dark: {
    accent: '#339cff',
    contrast: 60,
    fonts: { code: null, ui: null },
    ink: '#ffffff',
    opaqueWindows: false,
    semanticColors: { diffAdded: '#40c977', diffRemoved: '#fa423e', skill: '#ad7bf9' },
    surface: '#181818',
  },
}

const HEX = /^#[0-9a-fA-F]{6}$/

function isHex(v: unknown): v is string {
  return typeof v === 'string' && HEX.test(v)
}

function normalizeHex(v: string): string {
  return v.toLowerCase()
}

function clampContrast(v: unknown, fallback: number): number {
  if (typeof v !== 'number' || Number.isNaN(v)) return fallback
  return Math.min(100, Math.max(0, Math.round(v)))
}

function asNullableFont(v: unknown): string | null {
  if (typeof v !== 'string') return null
  const t = v.trim()
  return t.length > 0 ? t : null
}

function mergeChrome(partial: Partial<CodexChromeTheme> | undefined, base: CodexChromeTheme): CodexChromeTheme {
  const p = partial ?? {}
  const sc: Partial<CodexChromeTheme['semanticColors']> = p.semanticColors ?? {}
  return {
    accent: isHex(p.accent) ? normalizeHex(p.accent) : base.accent,
    contrast: clampContrast(p.contrast, base.contrast),
    fonts: {
      code: asNullableFont(p.fonts?.code) ?? base.fonts.code,
      ui: asNullableFont(p.fonts?.ui) ?? base.fonts.ui,
    },
    ink: isHex(p.ink) ? normalizeHex(p.ink) : base.ink,
    opaqueWindows: typeof p.opaqueWindows === 'boolean' ? p.opaqueWindows : base.opaqueWindows,
    semanticColors: {
      diffAdded: isHex(sc.diffAdded) ? normalizeHex(sc.diffAdded) : base.semanticColors.diffAdded,
      diffRemoved: isHex(sc.diffRemoved) ? normalizeHex(sc.diffRemoved) : base.semanticColors.diffRemoved,
      skill: isHex(sc.skill) ? normalizeHex(sc.skill) : base.semanticColors.skill,
    },
    surface: isHex(p.surface) ? normalizeHex(p.surface) : base.surface,
  }
}

export function chromeToScheme(theme: CodexChromeTheme): ColorScheme {
  return {
    accent: theme.accent,
    background: theme.surface,
    foreground: theme.ink,
    contrast: theme.contrast,
  }
}

export function schemeToChrome(scheme: ColorScheme, variant: CodexThemeVariant): CodexChromeTheme {
  const base = CODEX_DEFAULT_CHROME[variant]
  return {
    ...base,
    accent: normalizeHex(scheme.accent),
    surface: normalizeHex(scheme.background),
    ink: normalizeHex(scheme.foreground),
    contrast: clampContrast(scheme.contrast, base.contrast),
  }
}

export function exportCodexThemeString(
  scheme: ColorScheme,
  variant: CodexThemeVariant,
  codeThemeId = 'codex',
): string {
  const share: CodexThemeShare = {
    codeThemeId,
    theme: schemeToChrome(scheme, variant),
    variant,
  }
  return `${CODEX_THEME_PREFIX}${JSON.stringify(share)}`
}

export class CodexThemeParseError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'CodexThemeParseError'
  }
}

// Parse a Codex share string. When `expectedVariant` is set, reject mismatches
// the same way Codex does (light import into light panel only).
export function parseCodexThemeString(
  raw: string,
  expectedVariant?: CodexThemeVariant,
): { scheme: ColorScheme; share: CodexThemeShare } {
  const text = raw.trim()
  if (!text.startsWith(CODEX_THEME_PREFIX)) {
    throw new CodexThemeParseError('Not a codex-theme-v1 share string')
  }
  const payload = text.slice(CODEX_THEME_PREFIX.length)
  const jsonText = payload.startsWith('{') ? payload : decodeURIComponent(payload)
  let parsed: unknown
  try {
    parsed = JSON.parse(jsonText)
  } catch {
    throw new CodexThemeParseError('Invalid theme JSON')
  }
  if (!parsed || typeof parsed !== 'object') {
    throw new CodexThemeParseError('Invalid theme payload')
  }
  const obj = parsed as Record<string, unknown>
  const variant = obj.variant
  if (variant !== 'light' && variant !== 'dark') {
    throw new CodexThemeParseError('Missing theme variant')
  }
  if (expectedVariant != null && variant !== expectedVariant) {
    throw new CodexThemeParseError(`Theme is for ${variant}, not ${expectedVariant}`)
  }
  if (typeof obj.codeThemeId !== 'string' || obj.codeThemeId.length === 0) {
    throw new CodexThemeParseError('Missing code theme id')
  }
  const themeRaw = obj.theme
  if (!themeRaw || typeof themeRaw !== 'object') {
    throw new CodexThemeParseError('Missing theme colors')
  }
  const theme = mergeChrome(themeRaw as Partial<CodexChromeTheme>, CODEX_DEFAULT_CHROME[variant])
  const share: CodexThemeShare = {
    codeThemeId: obj.codeThemeId,
    theme,
    variant,
  }
  return { scheme: chromeToScheme(theme), share }
}

export function looksLikeCodexThemeString(raw: string): boolean {
  return raw.trim().startsWith(CODEX_THEME_PREFIX)
}
