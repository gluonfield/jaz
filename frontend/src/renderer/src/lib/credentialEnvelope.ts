export type StoredCredential = {
  token: string
  refreshToken?: string
}

export function encodeStoredCredential(token: string, refreshToken?: string): string {
  const refresh = refreshToken?.trim()
  if (!refresh) return token
  return JSON.stringify({ v: 2, token, refreshToken: refresh })
}

export function decodeStoredCredential(raw: string): StoredCredential {
  if (raw.startsWith('{')) {
    try {
      const parsed = JSON.parse(raw) as { v?: unknown; token?: unknown; refreshToken?: unknown }
      if (parsed.v === 2 && typeof parsed.token === 'string' && parsed.token.length > 0) {
        const refreshToken = typeof parsed.refreshToken === 'string' && parsed.refreshToken.trim()
          ? parsed.refreshToken.trim()
          : undefined
        return refreshToken ? { token: parsed.token, refreshToken } : { token: parsed.token }
      }
    } catch {
      // Fall back to legacy plain token string.
    }
  }
  return { token: raw }
}
