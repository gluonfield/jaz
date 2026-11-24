import { queryOptions } from '@tanstack/react-query'
import { ApiError } from '@/lib/api/response'
import { keys } from '@/lib/query/keys'
import {
  apiEmbeddedGetUrl as cloudEmbeddedGetUrl,
  del as cloudDel,
  get as cloudGet,
  patch as cloudPatch,
  post as cloudPost,
  put as cloudPut,
  secureBackendConnection,
} from './client'
import { sha256Hex } from './cloudUploads'

export type SpaceVisibility = 'private' | 'team' | 'selected'

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
  source_count?: number
  members?: CloudSpaceMember[]
}

export interface SpaceSource {
  id: string
  space_id: string
  org_id?: string
  uploaded_by?: string
  filename: string
  object_key?: string
  sha256?: string
  size_bytes: number
  content_type: string
  created_at: string
}

export interface ThreadUserState {
  thread_id?: string
  user_id?: string
  pinned?: boolean
  archived?: boolean
  unread?: boolean
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

export function getSpace(spaceId: string, signal?: AbortSignal): Promise<CloudSpace> {
  return cloudGet<CloudSpace>(`/v1/spaces/${encodeURIComponent(spaceId)}`, { signal })
}

export const spaceQuery = (spaceId: string) =>
  queryOptions({
    queryKey: keys.cloudSpace(spaceId),
    queryFn: ({ signal }) => getSpace(spaceId, signal),
    staleTime: 15_000,
  })

export async function createSpace(input: {
  name: string
  visibility?: SpaceVisibility
  member_user_ids?: string[]
}): Promise<CloudSpace> {
  const name = input.name.trim()
  if (!name) throw new Error('Project name is required')
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

export async function deleteSpace(spaceId: string): Promise<{ deleted: boolean; id: string }> {
  return cloudDel<{ deleted: boolean; id: string }>(`/v1/spaces/${encodeURIComponent(spaceId)}`)
}

export async function listSpaceSources(spaceId: string, signal?: AbortSignal): Promise<SpaceSource[]> {
  const response = await cloudGet<{ sources?: SpaceSource[] | null } | SpaceSource[]>(
    `/v1/spaces/${encodeURIComponent(spaceId)}/sources`,
    { signal },
  )
  return Array.isArray(response) ? response : (response.sources ?? [])
}

export const spaceSourcesQuery = (spaceId: string) =>
  queryOptions({
    queryKey: keys.cloudSpaceSources(spaceId),
    queryFn: ({ signal }) => listSpaceSources(spaceId, signal),
    staleTime: 15_000,
  })

export async function uploadSpaceSource(
  spaceId: string,
  file: File,
  signal?: AbortSignal,
): Promise<SpaceSource> {
  const contentType = file.type || 'application/octet-stream'
  const buffer = await file.arrayBuffer()
  const sha256 = await sha256Hex(buffer)
  const presigned = await cloudPost<{
    source_id: string
    filename: string
    object_key: string
    content_type: string
    put_url: string
    headers?: Record<string, string>
  }>(
    `/v1/spaces/${encodeURIComponent(spaceId)}/sources/presign`,
    {
      filename: file.name,
      content_type: contentType,
      sha256,
      size_bytes: buffer.byteLength,
    },
    signal,
  )
  const putUrl = presigned.put_url?.trim() ?? ''
  if (!putUrl || !secureBackendConnection(putUrl)) {
    throw new Error('Upload presign URL must use HTTPS or loopback')
  }
  const uploadHeaders = new Headers(presigned.headers)
  if (!uploadHeaders.has('Content-Type')) {
    uploadHeaders.set('Content-Type', presigned.content_type || contentType)
  }
  const putRes = await fetch(putUrl, {
    method: 'PUT',
    headers: uploadHeaders,
    body: buffer,
    credentials: 'omit',
    signal,
  })
  if (!putRes.ok) {
    throw new ApiError(putRes.status, `Upload storage rejected file (${putRes.status})`)
  }
  return cloudPost<SpaceSource>(
    `/v1/spaces/${encodeURIComponent(spaceId)}/sources/commit`,
    {
      source_id: presigned.source_id,
      filename: presigned.filename || file.name,
      object_key: presigned.object_key,
      sha256,
      size_bytes: buffer.byteLength,
      content_type: presigned.content_type || contentType,
    },
    signal,
  )
}

export async function deleteSpaceSource(spaceId: string, sourceId: string): Promise<{ deleted: boolean; id: string }> {
  return cloudDel<{ deleted: boolean; id: string }>(
    `/v1/spaces/${encodeURIComponent(spaceId)}/sources/${encodeURIComponent(sourceId)}`,
  )
}

export function spaceSourceContentUrl(spaceId: string, sourceId: string): string {
  return cloudEmbeddedGetUrl(
    `/v1/spaces/${encodeURIComponent(spaceId)}/sources/${encodeURIComponent(sourceId)}/content`,
  )
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
  patch: Partial<Pick<ThreadUserState, 'pinned' | 'archived' | 'unread' | 'last_read_seq' | 'personal_sources_enabled'>>,
): Promise<ThreadUserState> {
  const response = await cloudPut<RawThreadUserState>(`/v1/threads/${encodeURIComponent(threadId)}/user-state`, patch)
  return {
    thread_id: response?.thread_id ?? threadId,
    user_id: response?.user_id,
    pinned: response?.pinned ?? patch.pinned,
    archived: response?.archived ?? patch.archived,
    unread: response?.unread ?? patch.unread,
    last_read_seq: response?.last_read_seq ?? patch.last_read_seq,
    personal_sources_enabled:
      typeof response?.personal_sources_enabled === 'boolean'
        ? response.personal_sources_enabled
        : Boolean(patch.personal_sources_enabled),
  }
}
