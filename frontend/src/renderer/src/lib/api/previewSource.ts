export type PreviewProxyResponse = {
  url: string
}

type PreviewFetch = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>

const remotePreviewError =
  'Remote preview is unreachable. Check the server preview URL template and its DNS, TLS, and reverse-proxy routing.'

export async function preparePreviewProxySource(
  proxy: PreviewProxyResponse,
  fetcher: PreviewFetch = fetch,
): Promise<string> {
  if (!proxy.url) throw new Error('The Jaz backend did not return a preview URL.')
  let source: URL
  try {
    source = new URL(proxy.url)
  } catch {
    throw new Error(remotePreviewError)
  }
  if (!['http:', 'https:'].includes(source.protocol)) throw new Error(remotePreviewError)
  if (isLoopbackHostname(source.hostname)) return proxy.url
  const probe = new URL('/.well-known/jaz-preview', source)

  const controller = new AbortController()
  const timeout = setTimeout(() => controller.abort(), 8_000)
  try {
    const response = await fetcher(probe, {
      cache: 'no-store',
      credentials: 'omit',
      redirect: 'error',
      referrerPolicy: 'no-referrer',
      signal: controller.signal,
    })
    if (response.status !== 204 || response.headers.get('X-Jaz-Preview') !== 'ready') {
      throw new Error(remotePreviewError)
    }
    return proxy.url
  } catch {
    throw new Error(remotePreviewError)
  } finally {
    clearTimeout(timeout)
  }
}

export function isLoopbackHostname(hostname: string): boolean {
  const host = hostname.toLowerCase().replace(/^\[|\]$/g, '')
  return (
    host === 'localhost' ||
    host.endsWith('.localhost') ||
    host === '0.0.0.0' ||
    host === '::1' ||
    /^127(?:\.\d{1,3}){3}$/.test(host)
  )
}
