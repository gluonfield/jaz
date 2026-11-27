import type { AppBundleFile, AppDraft, AppExternalURL, AppKind } from '@/lib/api/apps'
import type { ACPToolCall } from '@/lib/api/types'

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

export function isAppDraftToolCall(call: Pick<ACPToolCall, 'tool_name' | 'title' | 'raw_input'>): boolean {
  if (isAppDraftToolName(call.tool_name) || isAppDraftToolName(call.title)) {
    return true
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

function extractDraftCandidate(raw: unknown): Record<string, unknown> | null {
  if (typeof raw === 'string') {
    const parsed = parseJsonString(raw)
    return parsed ? extractDraftCandidate(parsed) : null
  }
  const obj = asRecord(raw)
  if (!obj) return null

  if (typeof obj.draft_id === 'string' && typeof obj.bundle_sha256 === 'string') {
    return obj
  }
  if (obj.draft) {
    const nested = extractDraftCandidate(obj.draft)
    if (nested) return nested
  }
  if (obj.structuredContent) {
    const nested = extractDraftCandidate(obj.structuredContent)
    if (nested) return nested
  }
  if (obj.result) {
    const nested = extractDraftCandidate(obj.result)
    if (nested) return nested
  }
  if (Array.isArray(obj.content)) {
    for (const part of obj.content) {
      const rec = asRecord(part)
      if (rec && typeof rec.text === 'string') {
        const nested = extractDraftCandidate(rec.text)
        if (nested) return nested
      }
    }
  }
  return null
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
