import { apiBaseUrl, post } from './client'

const previewHosts = new Map<string, string>()
const previewIDs = new Map<string, string>()
const previewFallbacks = new Map<string, string>()

export async function resolvePreviewSource(value: string): Promise<string> {
  if (!shouldProxyPreview(value)) return value
  const response = await post<{ url: string; fallback_url?: string }>('/v1/preview/proxies', { url: value })
  const source = response.url || response.fallback_url || value
  rememberProxy(value, source)
  if (response.fallback_url) {
    rememberProxy(value, response.fallback_url)
    if (source !== response.fallback_url) previewFallbacks.set(source, response.fallback_url)
  }
  return source
}

export function takePreviewFallbackSource(value: string): string {
  const fallback = previewFallbacks.get(value) ?? ''
  if (fallback) previewFallbacks.delete(value)
  return fallback
}

export function previewDisplayUrl(value: string): string | null {
  let parsed: URL
  try {
    parsed = new URL(value)
  } catch {
    return null
  }
  const fromHost = previewHosts.get(parsed.host)
  if (fromHost) return `${fromHost}${parsed.pathname}${parsed.search}${parsed.hash}`
  if (!sameOrigin(parsed, apiBaseUrl())) return null
  const parts = parsed.pathname.split('/')
  if (parts.length >= 5 && parts[1] === 'v1' && parts[2] === 'preview' && parts[3] === 'p') {
    const origin = previewIDs.get(parts[4])
    if (!origin) return null
    const path = `/${parts.slice(5).join('/')}`
    return `${origin}${path}${parsed.search}${parsed.hash}`
  }
  return null
}

function rememberProxy(original: string, source: string): void {
  let originalURL: URL
  let sourceURL: URL
  try {
    originalURL = new URL(original)
    sourceURL = new URL(source)
  } catch {
    return
  }
  const originalOrigin = originalURL.origin
  const hostID = hostPreviewID(sourceURL.hostname)
  if (hostID) {
    previewHosts.set(sourceURL.host, originalOrigin)
    previewIDs.set(hostID, originalOrigin)
    return
  }
  const parts = sourceURL.pathname.split('/')
  if (parts.length >= 5 && parts[1] === 'v1' && parts[2] === 'preview' && parts[3] === 'p') {
    previewIDs.set(parts[4], originalOrigin)
  }
}

function isInternalPreviewUrl(value: string): boolean {
  try {
    const parsed = new URL(value)
    return sameOrigin(parsed, apiBaseUrl()) && parsed.pathname.startsWith('/v1/preview/')
  } catch {
    return false
  }
}

export function shouldProxyPreview(value: string): boolean {
  if (isInternalPreviewUrl(value)) return false
  try {
    const parsed = new URL(value)
    return (parsed.protocol === 'http:' || parsed.protocol === 'https:') && isLoopbackHost(parsed.hostname)
  } catch {
    return false
  }
}

function sameOrigin(url: URL, base: string): boolean {
  try {
    return url.origin === new URL(base).origin
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

function hostPreviewID(hostname: string): string {
  const label = hostname.toLowerCase().split('.')[0] ?? ''
  return label.startsWith('jaz-preview-') ? label.slice('jaz-preview-'.length) : ''
}
