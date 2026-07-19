import { post } from './client'
import { shouldProxyPreview } from '../../../../shared/preview'
import { preparePreviewProxySource, type PreviewProxyResponse } from './previewSource'

const previewHosts = new Map<string, string>()

export async function resolvePreviewSource(value: string): Promise<string> {
  if (!shouldProxyPreview(value)) return value
  const response = await post<PreviewProxyResponse>('/v1/preview/proxies', { url: value })
  const source = await preparePreviewProxySource(response)
  rememberProxy(value, source)
  return source
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
  previewHosts.set(sourceURL.host, originalOrigin)
}
