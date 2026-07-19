import { describe, expect, test } from 'bun:test'
import {
  CODEX_DEFAULT_CHROME,
  CODEX_THEME_PREFIX,
  CodexThemeParseError,
  chromeToScheme,
  exportCodexThemeString,
  parseCodexThemeString,
  schemeToChrome,
} from './codexTheme'

const sampleShare = {
  codeThemeId: 'catppuccin',
  theme: {
    accent: '#CBA6F7',
    contrast: 55,
    fonts: { code: 'JetBrains Mono', ui: 'Inter' },
    ink: '#CDD6F4',
    opaqueWindows: true,
    semanticColors: {
      diffAdded: '#A6E3A1',
      diffRemoved: '#F38BA8',
      skill: '#CBA6F7',
    },
    surface: '#1E1E2E',
  },
  variant: 'dark',
}

describe('parseCodexThemeString', () => {
  test('parses a Codex Copy theme string into Jaz scheme fields', () => {
    const raw = `${CODEX_THEME_PREFIX}${JSON.stringify(sampleShare)}`
    const { scheme, share } = parseCodexThemeString(raw)

    expect(scheme).toEqual({
      accent: '#cba6f7',
      background: '#1e1e2e',
      foreground: '#cdd6f4',
      contrast: 55,
    })
    expect(share.variant).toBe('dark')
    expect(share.codeThemeId).toBe('catppuccin')
    expect(share.theme.opaqueWindows).toBe(true)
    expect(share.theme.fonts.ui).toBe('Inter')
  })

  test('accepts URI-encoded payloads after the prefix', () => {
    const encoded = encodeURIComponent(JSON.stringify(sampleShare))
    const { scheme } = parseCodexThemeString(`${CODEX_THEME_PREFIX}${encoded}`, 'dark')
    expect(scheme.accent).toBe('#cba6f7')
  })

  test('fills missing chrome fields from Codex stock defaults', () => {
    const raw = `${CODEX_THEME_PREFIX}${JSON.stringify({
      codeThemeId: 'codex',
      variant: 'light',
      theme: { accent: '#ff0000', surface: '#fafafa', ink: '#111111', contrast: 30 },
    })}`
    const { scheme, share } = parseCodexThemeString(raw, 'light')
    expect(scheme).toEqual({
      accent: '#ff0000',
      background: '#fafafa',
      foreground: '#111111',
      contrast: 30,
    })
    expect(share.theme.semanticColors).toEqual(CODEX_DEFAULT_CHROME.light.semanticColors)
    expect(share.theme.opaqueWindows).toBe(false)
  })

  test('falls back cleanly for malformed nested chrome fields', () => {
    const raw = `${CODEX_THEME_PREFIX}${JSON.stringify({
      codeThemeId: 'codex',
      variant: 'dark',
      theme: { fonts: [], semanticColors: 'invalid' },
    })}`
    const { share } = parseCodexThemeString(raw, 'dark')
    expect(share.theme).toEqual(CODEX_DEFAULT_CHROME.dark)
  })

  test('rejects wrong variant when expected', () => {
    const raw = `${CODEX_THEME_PREFIX}${JSON.stringify(sampleShare)}`
    expect(() => parseCodexThemeString(raw, 'light')).toThrow(CodexThemeParseError)
  })

  test('rejects non-codex strings', () => {
    expect(() => parseCodexThemeString('{"accent":"#fff"}')).toThrow(/codex-theme-v1/)
  })
})

describe('exportCodexThemeString', () => {
  test('round-trips accent/background/foreground/contrast', () => {
    const scheme = {
      accent: '#339cff',
      background: '#181818',
      foreground: '#ffffff',
      contrast: 60,
    }
    const raw = exportCodexThemeString(scheme, 'dark')
    expect(raw.startsWith(CODEX_THEME_PREFIX)).toBe(true)
    const back = parseCodexThemeString(raw, 'dark')
    expect(back.scheme).toEqual(scheme)
    expect(back.share.variant).toBe('dark')
    expect(back.share.codeThemeId).toBe('codex')
  })

  test('exports stock Codex defaults as valid share strings', () => {
    for (const variant of ['light', 'dark']) {
      const scheme = chromeToScheme(CODEX_DEFAULT_CHROME[variant])
      const raw = exportCodexThemeString(scheme, variant)
      const { share } = parseCodexThemeString(raw, variant)
      expect(share.theme.accent).toBe(CODEX_DEFAULT_CHROME[variant].accent)
      expect(share.theme.surface).toBe(CODEX_DEFAULT_CHROME[variant].surface)
      expect(share.theme.ink).toBe(CODEX_DEFAULT_CHROME[variant].ink)
      expect(share.theme.contrast).toBe(CODEX_DEFAULT_CHROME[variant].contrast)
    }
  })
})

describe('schemeToChrome', () => {
  test('keeps Codex semantic defaults for the variant', () => {
    const chrome = schemeToChrome(
      { accent: '#abcdef', background: '#010101', foreground: '#fefefe', contrast: 70 },
      'dark',
    )
    expect(chrome.semanticColors).toEqual(CODEX_DEFAULT_CHROME.dark.semanticColors)
    expect(chrome.surface).toBe('#010101')
    expect(chrome.ink).toBe('#fefefe')
  })
})
