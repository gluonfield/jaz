const SCHEME_PATTERN = /^[a-z][a-z0-9+.-]*:\/\//i

export function isPreviewURL(value: string): boolean {
  return previewURL(value) !== null
}

export function shouldPreviewURLByDefault(value: string): boolean {
  const parsed = previewURL(value)
  return parsed !== null && isLoopbackHost(parsed.hostname)
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
