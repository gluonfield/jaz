export const CONNECTION_SESSION_CHANNEL = 'jaz:connection-session'
export const LOCAL_SESSION_CHANNEL = 'jaz:local-session'

export type ResourceTicket = { token: string; expires_at: string }
export type VerifiedSession = { url: string; token: string; deviceId: string; remembered: boolean }
export type LocalSession = VerifiedSession & { ticket?: ResourceTicket }
export type SessionSnapshot = {
  revision: number
  phase: 'initial' | 'pending' | 'connected' | 'blocked'
  url: string
  session: VerifiedSession | null
  error: string | null
  localPending: boolean
}
export type SessionDocument = { document: number; owner: boolean; snapshot: SessionSnapshot }
export type SessionMutation =
  | { operation: 'begin'; document: number; url: string }
  | { operation: 'commit'; document: number; revision: number; session: VerifiedSession }
  | { operation: 'clear'; document: number; error: string | null }
  | { operation: 'reject'; document: number; revision: number }
export type ConnectionSessionAPI = {
  read: () => SessionDocument
  update: (request: SessionMutation) => SessionSnapshot
  local: (document: number, revision: number) => Promise<LocalSession>
  subscribe: (handler: (snapshot: SessionSnapshot) => void) => () => void
}

export function validBackendOrigin(value: string): boolean {
  try {
    const url = new URL(value)
    return url.origin === value && (url.protocol === 'https:' ||
      (url.protocol === 'http:' && ['127.0.0.1', 'localhost', '[::1]'].includes(url.hostname)))
  } catch {
    return false
  }
}

export function deviceAccess(value: unknown, expectedDeviceId?: string): { deviceId: string; ticket?: ResourceTicket } {
  const body = value as { ok?: unknown; auth_kind?: unknown; device_id?: unknown; resource_ticket?: ResourceTicket } | null
  if (!body || body.ok !== true || body.auth_kind !== 'device' || typeof body.device_id !== 'string' || !body.device_id ||
    (expectedDeviceId !== undefined && body.device_id !== expectedDeviceId)) {
    throw new Error('Backend did not verify the expected device')
  }
  const ticket = body.resource_ticket
  if (ticket && (typeof ticket.token !== 'string' || !ticket.token || typeof ticket.expires_at !== 'string' || !(Date.parse(ticket.expires_at) > Date.now()))) {
    throw new Error('Backend returned an invalid resource ticket')
  }
  return { deviceId: body.device_id, ticket }
}
