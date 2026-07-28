export const DEFAULT_HOME_WORDMARK = 'jaz'
export const HOME_WORDMARK_MAX_LENGTH = 24

export function normalizeHomeWordmark(value: string): string {
  return Array.from(value.trim()).slice(0, HOME_WORDMARK_MAX_LENGTH).join('')
}

export function effectiveHomeWordmark(value: string): string {
  return normalizeHomeWordmark(value) || DEFAULT_HOME_WORDMARK
}
