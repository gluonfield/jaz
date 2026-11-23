import { queryOptions } from '@tanstack/react-query'
import { keys } from '@/lib/query/keys'
import { del, get, patch, post } from './client'

export type OrgMemberRole = 'admin' | 'member' | 'viewer'

export interface OrgMember {
  id: string
  org_id: string
  email: string
  display_name: string
  role: OrgMemberRole | string
  status: 'active' | 'invited' | string
  created_at: string
}

export async function listOrgMembers(
  query = '',
  limit = 50,
  signal?: AbortSignal,
): Promise<OrgMember[]> {
  const params = new URLSearchParams()
  const q = query.trim()
  if (q) params.set('q', q)
  if (limit > 0) params.set('limit', String(limit))
  const suffix = params.toString()
  const path = suffix ? `/v1/members?${suffix}` : '/v1/members'
  const res = await get<{ members?: OrgMember[] | null } | OrgMember[]>(path, { signal })
  if (Array.isArray(res)) return res
  return res.members ?? []
}

export const orgMembersQuery = (query = '', limit = 50) =>
  queryOptions({
    queryKey: keys.cloudMembers(query.trim()),
    queryFn: ({ signal }) => listOrgMembers(query, limit, signal),
    staleTime: 15_000,
  })

export function createOrgMember(input: {
  email: string
  display_name?: string
  role?: OrgMemberRole
}): Promise<OrgMember> {
  const email = input.email.trim().toLowerCase()
  if (!email) throw new Error('Email is required')
  const payload: { email: string; display_name?: string; role?: OrgMemberRole } = { email }
  if (input.display_name?.trim()) payload.display_name = input.display_name.trim()
  if (input.role) payload.role = input.role
  return post<OrgMember>('/v1/members', payload)
}

export function updateOrgMember(
  id: string,
  input: { display_name?: string; role?: OrgMemberRole },
): Promise<OrgMember> {
  return patch<OrgMember>(`/v1/members/${encodeURIComponent(id)}`, input)
}

export function deleteOrgMember(id: string): Promise<{ ok: boolean }> {
  return del<{ ok: boolean }>(`/v1/members/${encodeURIComponent(id)}`)
}

export function looksLikeEmail(raw: string): boolean {
  const value = raw.trim()
  if (!value || /\s/.test(value)) return false
  const at = value.indexOf('@')
  if (at <= 0 || at !== value.lastIndexOf('@')) return false
  const domain = value.slice(at + 1)
  return domain.includes('.') && !domain.startsWith('.') && !domain.endsWith('.')
}

export function filterOrgMembers(
  members: OrgMember[],
  query: string,
  excludeIds?: ReadonlySet<string>,
): OrgMember[] {
  const q = query.trim().toLowerCase()
  const filtered = members.filter((m) => {
    if (excludeIds?.has(m.id)) return false
    if (!q) return true
    return m.display_name.toLowerCase().includes(q) || m.email.toLowerCase().includes(q)
  })
  if (!q) return filtered
  return filtered.sort((a, b) => {
    const aPrefix =
      a.display_name.toLowerCase().startsWith(q) || a.email.toLowerCase().startsWith(q) ? 0 : 1
    const bPrefix =
      b.display_name.toLowerCase().startsWith(q) || b.email.toLowerCase().startsWith(q) ? 0 : 1
    if (aPrefix !== bPrefix) return aPrefix - bPrefix
    return (a.display_name || a.email).localeCompare(b.display_name || b.email)
  })
}
