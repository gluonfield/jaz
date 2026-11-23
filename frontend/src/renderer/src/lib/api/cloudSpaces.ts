import { queryOptions } from '@tanstack/react-query'
import { keys } from '@/lib/query/keys'
import { get as cloudGet, patch as cloudPatch, post as cloudPost, put as cloudPut } from './client'

export type SpaceVisibility = 'team' | 'selected'

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
  visibility?: SpaceVisibility
  created_by?: string
  can_edit?: boolean
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
  const response = await cloudGet<{ spaces?: CloudSpace[] | null } | CloudSpace[]>('/v1/spaces', { signal })
  if (Array.isArray(response)) return response
  return response.spaces ?? []
}

export const spacesQuery = queryOptions({
  queryKey: keys.cloudSpaces,
  queryFn: ({ signal }) => listSpaces(signal),
  staleTime: 15_000,
})

export async function createSpace(input: {
  name: string
  visibility?: SpaceVisibility
  member_user_ids?: string[]
}): Promise<CloudSpace> {
  const name = input.name.trim()
  if (!name) throw new Error('Space name is required')
  const payload: {
    name: string
    visibility?: SpaceVisibility
    member_user_ids?: string[]
  } = { name }
  if (input.visibility) payload.visibility = input.visibility
  if (input.member_user_ids) payload.member_user_ids = input.member_user_ids
  return cloudPost<CloudSpace>('/v1/spaces', payload)
}

export async function patchSpace(
  spaceId: string,
  input: {
    name?: string
    visibility?: SpaceVisibility
    member_user_ids?: string[]
  },
): Promise<CloudSpace> {
  return cloudPatch<CloudSpace>(`/v1/spaces/${encodeURIComponent(spaceId)}`, input)
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
  const response = await cloudPost<CloudSpaceMember>(`/v1/spaces/${encodeURIComponent(spaceId)}/members`, payload)
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
  const response = await cloudPut<RawThreadUserState>(`/v1/threads/${encodeURIComponent(threadId)}/user-state`, patch)
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
