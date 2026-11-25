import { protocol, type Session } from 'electron'
import { LOCAL_BACKEND_SCHEME, LOCAL_BACKEND_TRANSPORT_URL } from '@shared/localBackend'

export function registerLocalBackendTransport(): void {
  protocol.registerSchemesAsPrivileged([{
    scheme: LOCAL_BACKEND_SCHEME,
    privileges: { standard: true, secure: true, supportFetchAPI: true, corsEnabled: true, stream: true },
  }])
}

const ASSET_GRANT_PATH = /^\/v1\/sessions\/[^/]+\/assets\/.+$/

export function isSandboxedAssetGrantRequest(
  url: URL,
  request: { method: string; initiatorOrigin?: unknown },
): boolean {
  if (request.initiatorOrigin !== 'null') return false
  if (request.method !== 'GET' && request.method !== 'HEAD' && request.method !== 'OPTIONS') return false
  if (!ASSET_GRANT_PATH.test(url.pathname) || /%(?:2e|2f|5c)/i.test(url.pathname)) return false
  const grant = url.searchParams.get('grant')
  return Boolean(grant && grant.startsWith('ag_'))
}

export function configureLocalBackendTransport(target: Session, backendURL: string): void {
  const rendererURL = process.env.ELECTRON_RENDERER_URL
  const origin = rendererURL ? new URL(rendererURL).origin : 'file://'
  target.protocol.handle(LOCAL_BACKEND_SCHEME, async (request) => {
    const url = new URL(request.url)
    if (url.host !== 'backend' || url.username || url.password || !('initiatorOrigin' in request)) {
      return new Response('Forbidden', { status: 403 })
    }
    const fromRenderer = request.initiatorOrigin === origin
    const fromSandboxedAsset = !fromRenderer && isSandboxedAssetGrantRequest(url, request)
    if (!fromRenderer && !fromSandboxedAsset) {
      return new Response('Forbidden', { status: 403 })
    }
    const targetURL = backendURL + url.pathname + url.search
    if (fromSandboxedAsset) {
      const headers = new Headers(request.headers)
      headers.delete('Authorization')
      headers.delete('Cookie')
      const response = await fetch(targetURL, {
        method: request.method,
        headers,
        redirect: 'manual',
      })
      if ([301, 302, 303, 307, 308].includes(response.status)) {
        await response.body?.cancel()
        return Response.error()
      }
      return response
    }
    // Node's loopback transport stays available when Chromium reports the computer offline.
    const response = await fetch(new Request(targetURL, request), { redirect: 'manual' })
    const location = response.headers.get('Location')
    if (!location || ![301, 302, 303, 307, 308].includes(response.status)) return response
    const redirect = new URL(location, targetURL)
    if (redirect.origin !== new URL(backendURL).origin) {
      await response.body?.cancel()
      return Response.error()
    }
    const headers = new Headers(response.headers)
    headers.set('Location', LOCAL_BACKEND_TRANSPORT_URL + redirect.pathname + redirect.search + redirect.hash)
    return new Response(response.body, { status: response.status, statusText: response.statusText, headers })
  })
}
