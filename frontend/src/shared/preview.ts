export function isPreviewURL(value: string): boolean {
  try {
    const parsed = new URL(value)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:'
  } catch {
    return false
  }
}

export function normalizePreviewURL(value: string): string {
  const trimmed = value.trim()
  if (!trimmed) return ''
  const withScheme = /^[a-z][a-z0-9+.-]*:\/\//i.test(trimmed)
    ? trimmed
    : /^(localhost|127\.0\.0\.1|0\.0\.0\.0|\[::1\])(?::|\/|$)/i.test(trimmed)
      ? `http://${trimmed}`
      : `https://${trimmed}`
  try {
    const parsed = new URL(withScheme)
    return isPreviewURL(parsed.toString()) ? parsed.toString() : ''
  } catch {
    return ''
  }
}
