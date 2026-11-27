import type { AppBundleFile, AppDraft, AppExternalURL, AppKind } from '@/lib/api/apps'
import type { ACPToolCall } from '@/lib/api/types'

const MAX_EXTRACT_DEPTH = 6

export function isAppDraftToolName(name?: string): boolean {
  if (!name) return false
  return (
    name === 'prepare_app_draft' ||
    name === 'mcp_jaztools_prepare_app_draft' ||
    name === 'mcp__jaztools__prepare_app_draft' ||
    name === 'mcp.jaztools.prepare_app_draft' ||
    name.endsWith('.prepare_app_draft') ||
    name.endsWith('__prepare_app_draft')
  )
}

function isDeferredCallToolName(name?: string): boolean {
  if (!name) return false
  return (
    name === 'call_tool' ||
    name.endsWith('.call_tool') ||
    name.endsWith('__call_tool') ||
    name.endsWith('_call_tool')
  )
}

export function isAppDraftToolCall(call: Pick<ACPToolCall, 'tool_name' | 'title' | 'raw_input'>): boolean {
  if (isAppDraftToolName(call.tool_name) || isAppDraftToolName(call.title)) {
    return true
  }
  if (!isDeferredCallToolName(call.tool_name) && !isDeferredCallToolName(call.title)) {
    return false
  }
  const input = call.raw_input
  if (input && typeof input === 'object' && 'name' in input) {
    const wrapped = (input as { name?: unknown }).name
    if (typeof wrapped === 'string' && isAppDraftToolName(wrapped)) {
      return true
    }
  }
  return false
}

function asRecord(v: unknown): Record<string, unknown> | null {
  return v && typeof v === 'object' && !Array.isArray(v) ? (v as Record<string, unknown>) : null
}

function parseJsonString(raw: string): unknown {
  try {
    return JSON.parse(raw)
  } catch {
    return null
  }
}

function extractDraftCandidate(raw: unknown, depth = 0): Record<string, unknown> | null {
  if (depth > MAX_EXTRACT_DEPTH || raw == null) return null
  if (typeof raw === 'string') {
    const parsed = parseJsonString(raw)
    return parsed ? extractDraftCandidate(parsed, depth + 1) : null
  }
  const obj = asRecord(raw)
  if (!obj) return null
  if (obj.isError === true || (obj.error != null && obj.error !== '' && obj.error !== false)) {
    return null
  }

  if (typeof obj.draft_id === 'string' && typeof obj.bundle_sha256 === 'string') {
    return obj
  }
  for (const key of ['draft', 'structuredContent', 'result'] as const) {
    if (obj[key] != null) {
      const nested = extractDraftCandidate(obj[key], depth + 1)
      if (nested) return nested
    }
  }
  if (Array.isArray(obj.content)) {
    for (const part of obj.content) {
      const rec = asRecord(part)
      if (rec && typeof rec.text === 'string') {
        const nested = extractDraftCandidate(rec.text, depth + 1)
        if (nested) return nested
      }
    }
  }
  return null
}

export function extractAppDraftToolError(rawOutput: unknown, depth = 0): string {
  if (depth > MAX_EXTRACT_DEPTH || rawOutput == null) return ''
  if (typeof rawOutput === 'string') {
    const trimmed = rawOutput.trim()
    if (!trimmed) return ''
    const parsed = parseJsonString(trimmed)
    if (parsed && typeof parsed === 'object') {
      return extractAppDraftToolError(parsed, depth + 1)
    }
    return trimmed
  }
  const obj = asRecord(rawOutput)
  if (!obj) return ''
  if (typeof obj.error === 'string' && obj.error.trim()) return obj.error.trim()
  const errRec = asRecord(obj.error)
  if (errRec && typeof errRec.message === 'string' && errRec.message.trim()) return errRec.message.trim()
  if (obj.result != null) {
    const nested = extractAppDraftToolError(obj.result, depth + 1)
    if (nested) return nested
  }
  if (Array.isArray(obj.content)) {
    const texts = obj.content
      .map((part) => asRecord(part))
      .filter((part): part is Record<string, unknown> => Boolean(part && typeof part.text === 'string'))
      .map((part) => String(part.text).trim())
      .filter(Boolean)
    if (texts.length > 0) return texts.join(' ')
  }
  return ''
}

export function isAppDraftExpired(expiresAt?: string, nowMs = Date.now()): boolean {
  if (!expiresAt) return false
  const parsed = Date.parse(expiresAt)
  return Number.isFinite(parsed) && parsed <= nowMs
}

export function parseAppDraftToolOutput(rawOutput: unknown): AppDraft | null {
  const c = extractDraftCandidate(rawOutput)
  if (!c) return null
  const draftId = typeof c.draft_id === 'string' ? c.draft_id.trim() : ''
  const sha = typeof c.bundle_sha256 === 'string' ? c.bundle_sha256.trim() : ''
  if (!draftId || !sha) return null

  const kind: AppKind = c.kind === 'document' ? 'document' : 'artifact'
  const files: AppBundleFile[] = Array.isArray(c.files)
    ? c.files
        .map((f) => asRecord(f))
        .filter((f): f is Record<string, unknown> => Boolean(f && typeof f.path === 'string'))
        .map((f) => ({
          path: String(f.path),
          size_bytes: typeof f.size_bytes === 'number' ? f.size_bytes : 0,
          sha256: typeof f.sha256 === 'string' ? f.sha256 : '',
          content_type: typeof f.content_type === 'string' ? f.content_type : 'application/octet-stream',
        }))
    : []

  const externalUrls: AppExternalURL[] = Array.isArray(c.external_urls)
    ? c.external_urls
        .map((u) => asRecord(u))
        .filter((u): u is Record<string, unknown> => Boolean(u && typeof u.url === 'string'))
        .map((u) => ({
          url: String(u.url),
          host: typeof u.host === 'string' ? u.host : '',
          allowlisted: Boolean(u.allowlisted),
          source_file: typeof u.source_file === 'string' ? u.source_file : undefined,
        }))
    : []

  return {
    draft_id: draftId,
    target_app_id: typeof c.target_app_id === 'string' && c.target_app_id ? c.target_app_id : undefined,
    title: typeof c.title === 'string' && c.title.trim() ? c.title.trim() : 'Untitled App',
    description: typeof c.description === 'string' ? c.description : undefined,
    kind,
    bundle_sha256: sha,
    entry_path: typeof c.entry_path === 'string' ? c.entry_path : kind === 'document' ? 'index.md' : 'index.html',
    size_bytes: typeof c.size_bytes === 'number' ? c.size_bytes : 0,
    file_count: typeof c.file_count === 'number' ? c.file_count : files.length,
    capabilities: Array.isArray(c.capabilities) ? c.capabilities.filter((x): x is string => typeof x === 'string') : [],
    allowed_tools: Array.isArray(c.allowed_tools) ? c.allowed_tools.filter((x): x is string => typeof x === 'string') : [],
    external_urls: externalUrls,
    warnings: Array.isArray(c.warnings) ? c.warnings.filter((x): x is string => typeof x === 'string') : [],
    files,
    expires_at: typeof c.expires_at === 'string' ? c.expires_at : '',
    created_at: typeof c.created_at === 'string' ? c.created_at : new Date().toISOString(),
  }
}
