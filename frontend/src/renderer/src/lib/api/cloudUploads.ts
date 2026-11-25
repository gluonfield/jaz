import { ApiError, readAPIResponse } from '@/lib/api/response'
import type { ResolvedAsset } from '@/lib/artifacts'
import {
  apiEmbeddedGetUrl as cloudEmbeddedGetUrl,
  apiFetch as cloudFetch,
  cachedResourceTicket,
  post as cloudPost,
  secureBackendConnection,
} from './client'
import type { Attachment, SessionFileRead } from './types'

const CLOUD_FILE_READ_LIMIT = 1024 * 1024

export interface AssetGrantWire {
  url: string
  size?: number
  size_bytes?: number
  content_type?: string
  contentType?: string
  etag?: string
  expires_at?: string
  expiresAt?: string
}

export function normalizeAssetGrant(wire: AssetGrantWire, url: string): ResolvedAsset {
  const size = wire.size ?? wire.size_bytes ?? 0
  const contentType = wire.content_type ?? wire.contentType ?? 'application/octet-stream'
  const expiresAt = wire.expires_at ?? wire.expiresAt ?? ''
  return {
    url,
    size,
    contentType,
    ...(wire.etag ? { etag: wire.etag } : {}),
    expiresAt,
  }
}

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
  return cloudPost<PresignUploadResponse>(`/v1/runs/${encodeURIComponent(runId)}/uploads/presign`, input, signal)
}

export function commitUpload(runId: string, input: CommitUploadInput, signal?: AbortSignal): Promise<CommitUploadResponse> {
  return cloudPost<CommitUploadResponse>(`/v1/runs/${encodeURIComponent(runId)}/uploads/commit`, input, signal)
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

export function normalizeCloudRunRelpath(rawPath: string): string {
  let value = rawPath.trim()
  if (/^file:\/\//i.test(value)) {
    value = value.replace(/^file:\/\//i, '')
    try {
      value = decodeURIComponent(value)
    } catch {
      // Keep raw path when percent-encoding is malformed.
    }
  }
  value = value.replace(/[?#].*$/, '').replace(/\\/g, '/')
  if (value === '/workspace') {
    value = ''
  } else if (value.startsWith('/workspace/')) {
    value = value.slice('/workspace/'.length)
  }
  const parts: string[] = []
  for (const segment of value.replace(/^\/+/, '').split('/')) {
    if (!segment || segment === '.') continue
    if (segment === '..') {
      if (parts.length === 0) {
        throw new ApiError(400, 'invalid file path')
      }
      parts.pop()
      continue
    }
    parts.push(segment)
  }
  if (parts.length === 0) {
    throw new ApiError(400, 'file path is required')
  }
  return parts.join('/')
}

export function cloudRunFilePath(runId: string, relpath: string): string {
  const encodedPath = normalizeCloudRunRelpath(relpath)
    .split('/')
    .map((segment) => encodeURIComponent(segment))
    .join('/')
  return `/v1/runs/${encodeURIComponent(runId)}/files/${encodedPath}`
}

export function cloudRunFileUrl(runId: string, relpath: string): string {
  return cloudEmbeddedGetUrl(cloudRunFilePath(runId, relpath))
}

export function cloudRunFileDownloadUrl(runId: string, relpath: string): string {
  return cloudEmbeddedGetUrl(`${cloudRunFilePath(runId, relpath)}?download=1`)
}

function hasActiveResourceTicket(): boolean {
  const ticket = cachedResourceTicket()
  if (!ticket?.token) return false
  if (!ticket.expires_at) return true
  const expiresAt = Date.parse(ticket.expires_at)
  return Number.isNaN(expiresAt) || expiresAt > Date.now()
}

async function fetchCloudRunFile(
  runId: string,
  relpath: string,
  options: { signal?: AbortSignal; headers?: HeadersInit } = {},
): Promise<Response> {
  if (hasActiveResourceTicket()) {
    const response = await fetch(cloudRunFileUrl(runId, relpath), {
      signal: options.signal,
      headers: options.headers,
      credentials: 'omit',
    })
    if (response.status !== 401) return response
  }
  return cloudFetch(cloudRunFilePath(runId, relpath), { signal: options.signal, headers: options.headers })
}

export async function grantCloudRunAsset(runId: string, relpath: string, signal?: AbortSignal): Promise<ResolvedAsset> {
  const cleanRelpath = normalizeCloudRunRelpath(relpath)
  const wire = await cloudPost<AssetGrantWire>(
    `/v1/runs/${encodeURIComponent(runId)}/assets/grant`,
    { path: cleanRelpath },
    signal,
  )
  const url = wire.url?.trim() ?? ''
  if (!url || !secureBackendConnection(url)) {
    throw new Error('Asset grant URL must use HTTPS or loopback')
  }
  return normalizeAssetGrant(wire, url)
}

export async function cloudRunFileBytes(runId: string, relpath: string, signal?: AbortSignal): Promise<ArrayBuffer> {
  const response = await fetchCloudRunFile(runId, relpath, { signal })
  if (!response.ok) await readAPIResponse(response)
  return response.arrayBuffer()
}

function parseContentRangeTotal(header: string | null): number | null {
  if (!header) return null
  const match = /^bytes\s+(?:\d+-\d+|\*)\/(\d+)$/i.exec(header.trim())
  if (!match) return null
  const total = Number(match[1])
  return Number.isSafeInteger(total) ? total : null
}

export async function readCloudRunFile(runId: string, relpath: string, signal?: AbortSignal): Promise<SessionFileRead> {
  const cleanRelpath = normalizeCloudRunRelpath(relpath)
  const response = await fetchCloudRunFile(runId, cleanRelpath, {
    signal,
    headers: { Range: `bytes=0-${CLOUD_FILE_READ_LIMIT}` },
  })
  const etag = response.headers.get('ETag')
  if (response.status === 416 && parseContentRangeTotal(response.headers.get('Content-Range')) === 0) {
    return {
      path: cleanRelpath,
      relative_path: cleanRelpath,
      size: 0,
      binary: false,
      truncated: false,
      content: '',
      ...(etag ? { version: etag } : {}),
      read_only: true,
    }
  }
  if (!response.ok) await readAPIResponse(response)
  const buffer = await response.arrayBuffer()
  const rangeTotal = response.status === 206 ? parseContentRangeTotal(response.headers.get('Content-Range')) : null
  const totalSize = rangeTotal ?? buffer.byteLength
  const truncated = totalSize > CLOUD_FILE_READ_LIMIT || buffer.byteLength > CLOUD_FILE_READ_LIMIT
  const sample = new Uint8Array(buffer.byteLength > CLOUD_FILE_READ_LIMIT ? buffer.slice(0, CLOUD_FILE_READ_LIMIT) : buffer)
  let binary = sample.includes(0)
  let content: string | undefined
  if (!binary) {
    try {
      content = new TextDecoder('utf-8', { fatal: true }).decode(sample)
    } catch {
      binary = true
    }
  }
  return {
    path: cleanRelpath,
    relative_path: cleanRelpath,
    size: totalSize,
    binary,
    truncated,
    ...(binary ? {} : { content }),
    ...(etag ? { version: etag } : {}),
    read_only: true,
  }
}
