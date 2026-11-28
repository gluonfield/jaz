export const CONNECTION_SESSION_CHANNEL = 'jaz:connection-session'
export const LOCAL_SESSION_CHANNEL = 'jaz:local-session'

export type AuthKind = 'bearer' | 'oidc' | 'device'
export type ResourceTicket = { token: string; expires_at: string }
export type VerifiedSession = { url: string; token: string; remembered: boolean }
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
  read: () => SessionDocument | null
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

export function verifiedAccess(value: unknown): { kind: AuthKind; ticket?: ResourceTicket } {
  const body = value as { ok?: unknown; auth_kind?: unknown; resource_ticket?: ResourceTicket } | null
  const kind = body?.auth_kind
  if (!body || body.ok !== true || (kind !== 'bearer' && kind !== 'oidc' && kind !== 'device')) {
    throw new Error('Backend did not verify the session credential')
  }
  const ticket = body.resource_ticket
  if (ticket && (typeof ticket.token !== 'string' || !ticket.token || typeof ticket.expires_at !== 'string' || !(Date.parse(ticket.expires_at) > Date.now()))) {
    throw new Error('Backend returned an invalid resource ticket')
  }
  return { kind, ticket }
}
