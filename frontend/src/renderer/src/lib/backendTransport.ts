import { clientRuntime } from '@/lib/clientRuntime'
import { LOCAL_BACKEND_URL } from '@shared/localBackend'

export function backendTransportUrl(url: string): string {
  const transport = clientRuntime.localBackendTransport
  return transport && url.startsWith(LOCAL_BACKEND_URL + '/')
    ? transport + url.slice(LOCAL_BACKEND_URL.length)
    : url
}

export function backendFetch(url: string, init?: RequestInit): Promise<Response> {
  return fetch(backendTransportUrl(url), init)
}
