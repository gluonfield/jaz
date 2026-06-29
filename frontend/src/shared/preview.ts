const SCHEME_PATTERN = /^[a-z][a-z0-9+.-]*:\/\//i

export function isPreviewURL(value: string): boolean {
  return previewURL(value) !== null
}

export function shouldPreviewURLByDefault(value: string): boolean {
  return isPreviewURL(value)
}

function previewURL(value: string): URL | null {
  try {
    const parsed = new URL(value)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:' ? parsed : null
  } catch {
    return null
  }
}

function isLoopbackInput(value: string): boolean {
  try {
    return isLoopbackHost(new URL(`http://${value}`).hostname)
  } catch {
    return false
  }
}

function isLoopbackHost(hostname: string): boolean {
  const host = hostname.toLowerCase().replace(/^\[|\]$/g, '')
  return (
    host === 'localhost' ||
    host === '0.0.0.0' ||
    host === '::1' ||
    /^127(?:\.\d{1,3}){3}$/.test(host)
  )
}

const DEFAULT_PREVIEW_PATTERNS = ['localhost', '127\\.0\\.0\\.1']

export function resolvePreviewPatterns(configured?: readonly string[]): string[] {
  const cleaned = (configured ?? []).map((pattern) => pattern.trim()).filter(Boolean)
  return cleaned.length ? cleaned : [...DEFAULT_PREVIEW_PATTERNS]
}

export function matchesPreviewPattern(value: string, patterns: readonly string[]): boolean {
  const parsed = previewURL(value)
  if (!parsed) return false
  return patterns.some((pattern) => testPreviewPattern(pattern, parsed.href))
}

function testPreviewPattern(pattern: string, href: string): boolean {
  try {
    return new RegExp(pattern, 'i').test(href)
  } catch {
    return false
  }
}

const URL_IN_TEXT = /https?:\/\/[^\s<>()[\]"'`]+/gi

// Pull http(s) URLs from assistant prose, keep those matching a preview pattern,
// deduped in first-seen order. Trailing delimiters/punctuation are shaved.
export function findPreviewURLs(text: string, patterns: readonly string[]): string[] {
  const matches = text.match(URL_IN_TEXT)
  if (!matches) return []
  const seen = new Set<string>()
  const urls: string[] = []
  for (const raw of matches) {
    const url = raw.replace(/[\\.,;:!?)\]}'"*_]+$/, '')
    const parsed = previewURL(url)
    if (!parsed || seen.has(parsed.href) || !patterns.some((pattern) => testPreviewPattern(pattern, parsed.href))) continue
    seen.add(parsed.href)
    urls.push(url)
  }
  return urls
}

export function normalizePreviewURL(value: string): string {
  const trimmed = value.trim()
  if (!trimmed) return ''
  const withScheme = SCHEME_PATTERN.test(trimmed)
    ? trimmed
    : isLoopbackInput(trimmed)
      ? `http://${trimmed}`
      : `https://${trimmed}`
  return previewURL(withScheme)?.toString() ?? ''
}
