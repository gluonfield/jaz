import { ApiError } from '@/lib/api/response'
import { apiEmbeddedGetUrl, post, secureBackendConnection } from './client'
import type { Attachment } from './types'

export interface PresignUploadInput {
  relpath: string
  content_type: string
  sha256: string
}

export interface PresignUploadResponse {
  input_id: string
  relpath: string
  object_key: string
  content_type: string
  put_url: string
  headers?: Record<string, string>
}

export interface CommitUploadInput {
  input_id: string
  relpath: string
  object_key: string
  sha256: string
  size_bytes: number
  content_type: string
}

export interface CommitUploadResponse {
  run_id?: string
  upload_seq?: number
  input_id?: string
  relpath?: string
  object_key?: string
  sha256?: string
  size_bytes?: number
  content_type?: string
  created_at?: string
  attachment?: Attachment
}

export function presignUpload(runId: string, input: PresignUploadInput, signal?: AbortSignal): Promise<PresignUploadResponse> {
  return post<PresignUploadResponse>(`/v1/runs/${encodeURIComponent(runId)}/uploads/presign`, input, signal)
}

export function commitUpload(runId: string, input: CommitUploadInput, signal?: AbortSignal): Promise<CommitUploadResponse> {
  return post<CommitUploadResponse>(`/v1/runs/${encodeURIComponent(runId)}/uploads/commit`, input, signal)
}

export async function sha256Hex(data: ArrayBuffer): Promise<string> {
  const digest = await crypto.subtle.digest('SHA-256', data)
  return Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, '0')).join('')
}

export async function uploadCloudFile(
  runId: string,
  file: File,
  options: { relpath?: string; signal?: AbortSignal; onProgress?: (fraction: number) => void } = {},
): Promise<Attachment> {
  const relpath = options.relpath?.trim() || file.name
  const contentType = file.type || 'application/octet-stream'
  const buffer = await file.arrayBuffer()
  const sha256 = await sha256Hex(buffer)
  options.onProgress?.(0.1)

  const presigned = await presignUpload(
    runId,
    { relpath, content_type: contentType, sha256 },
    options.signal,
  )
  const putUrl = presigned.put_url?.trim() ?? ''
  if (!putUrl || !secureBackendConnection(putUrl)) {
    throw new Error('Upload presign URL must use HTTPS or loopback')
  }
  options.onProgress?.(0.25)

  const uploadHeaders = new Headers(presigned.headers)
  if (!uploadHeaders.has('Content-Type')) {
    uploadHeaders.set('Content-Type', presigned.content_type || contentType)
  }
  const putRes = await fetch(putUrl, {
    method: 'PUT',
    headers: uploadHeaders,
    body: buffer,
    credentials: 'omit',
    signal: options.signal,
  })
  if (!putRes.ok) {
    throw new ApiError(putRes.status, `Upload storage rejected file (${putRes.status})`)
  }
  options.onProgress?.(0.85)

  const finalRelpath = presigned.relpath || relpath
  const committed = await commitUpload(
    runId,
    {
      input_id: presigned.input_id,
      relpath: finalRelpath,
      object_key: presigned.object_key,
      sha256,
      size_bytes: buffer.byteLength,
      content_type: presigned.content_type || contentType,
    },
    options.signal,
  )
  options.onProgress?.(1)

  if (committed.attachment) {
    return committed.attachment
  }
  const committedRelpath = committed.relpath || finalRelpath
  return {
    id: committed.input_id || presigned.input_id || committed.object_key || presigned.object_key,
    name: file.name,
    uri: cloudRunFileUrl(runId, committedRelpath),
    mime_type: committed.content_type || contentType,
    size: buffer.byteLength,
  }
}

export function cloudRunFileUrl(runId: string, relpath: string): string {
  const encodedPath = relpath
    .replace(/^\/+/, '')
    .split('/')
    .map((segment) => encodeURIComponent(segment))
    .join('/')
  return apiEmbeddedGetUrl(`/v1/runs/${encodeURIComponent(runId)}/files/${encodedPath}`)
}
