import { queryOptions } from '@tanstack/react-query'
import { keys } from '@/lib/query/keys'
import { get, post, put } from './client'

export interface CloudSpaceMember {
  space_id?: string
  user_id?: string
  email?: string
  display_name?: string
  role?: string
  ok?: boolean
}

export interface CloudSpace {
  id: string
  org_id?: string
  name: string
  created_by?: string
  created_at?: string
  member_count?: number
  members?: CloudSpaceMember[]
}

export interface ThreadUserState {
  thread_id?: string
  user_id?: string
  pinned?: boolean
  archived?: boolean
  last_read_seq?: number
  personal_sources_enabled: boolean
}

export async function listSpaces(signal?: AbortSignal): Promise<CloudSpace[]> {
  const response = await get<{ spaces?: CloudSpace[] | null } | CloudSpace[]>('/v1/spaces', { signal })
  if (Array.isArray(response)) return response
  return response.spaces ?? []
}

export const spacesQuery = queryOptions({
  queryKey: keys.cloudSpaces,
  queryFn: ({ signal }) => listSpaces(signal),
  staleTime: 15_000,
})

export async function createSpace(input: { name: string }): Promise<CloudSpace> {
  const name = input.name.trim()
  if (!name) throw new Error('Space name is required')
  return post<CloudSpace>('/v1/spaces', { name })
}

export async function addSpaceMember(
  spaceId: string,
  input: { user_id?: string; email?: string; role?: string },
): Promise<CloudSpaceMember> {
  const payload: { user_id?: string; email?: string; role: string } = {
    role: input.role?.trim() || 'member',
  }
  if (input.email?.trim()) payload.email = input.email.trim()
  if (input.user_id?.trim()) payload.user_id = input.user_id.trim()
  if (!payload.email && !payload.user_id) {
    throw new Error('Enter a user email or ID')
  }
  const response = await post<CloudSpaceMember>(`/v1/spaces/${encodeURIComponent(spaceId)}/members`, payload)
  return {
    space_id: response?.space_id ?? spaceId,
    user_id: response?.user_id ?? payload.user_id,
    email: response?.email ?? payload.email,
    display_name: response?.display_name,
    role: response?.role ?? payload.role,
    ...(response?.ok !== undefined ? { ok: response.ok } : {}),
  }
}

type RawThreadUserState = Partial<ThreadUserState> & { ok?: boolean }

export async function updateThreadUserState(
  threadId: string,
  patch: Partial<Pick<ThreadUserState, 'pinned' | 'archived' | 'last_read_seq' | 'personal_sources_enabled'>>,
): Promise<ThreadUserState> {
  const response = await put<RawThreadUserState>(`/v1/threads/${encodeURIComponent(threadId)}/user-state`, patch)
  return {
    thread_id: response?.thread_id ?? threadId,
    user_id: response?.user_id,
    pinned: response?.pinned ?? patch.pinned,
    archived: response?.archived ?? patch.archived,
    last_read_seq: response?.last_read_seq ?? patch.last_read_seq,
    personal_sources_enabled:
      typeof response?.personal_sources_enabled === 'boolean'
        ? response.personal_sources_enabled
        : Boolean(patch.personal_sources_enabled),
  }
}
