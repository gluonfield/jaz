import type { AppKind, PendingAppDraft } from '@/lib/api/apps'
import type { ACPToolCall } from '@/lib/api/types'
import type { PublishedAppRef } from '@/lib/sidePanelTabs'

// What an app tool returned: publish_app's saved app, or prepare_app_draft's
// staged draft waiting for the user.
export type AppToolResult = { app: PublishedAppRef & { kind: AppKind } } | { draft: PendingAppDraft }

const MAX_EXTRACT_DEPTH = 6

const APP_TOOLS = ['publish_app', 'prepare_app_draft']

export function isAppToolName(name?: string): boolean {
  if (!name) return false
  return APP_TOOLS.some((tool) =>
    name === tool || name === `mcp_jaztools_${tool}` || name.endsWith(`.${tool}`) || name.endsWith(`__${tool}`))
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

export function isAppToolCall(call: Pick<ACPToolCall, 'tool_name' | 'title' | 'raw_input'>): boolean {
  if (isAppToolName(call.tool_name) || isAppToolName(call.title)) {
    return true
  }
  if (!isDeferredCallToolName(call.tool_name) && !isDeferredCallToolName(call.title)) {
    return false
  }
  const input = call.raw_input
  if (input && typeof input === 'object' && 'name' in input) {
    const wrapped = (input as { name?: unknown }).name
    if (typeof wrapped === 'string' && isAppToolName(wrapped)) {
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

function extractCandidate(raw: unknown, depth = 0): Record<string, unknown> | null {
  if (depth > MAX_EXTRACT_DEPTH || raw == null) return null
  if (typeof raw === 'string') {
    const parsed = parseJsonString(raw)
    return parsed ? extractCandidate(parsed, depth + 1) : null
  }
  const obj = asRecord(raw)
  if (!obj) return null
  if (obj.isError === true || (obj.error != null && obj.error !== '' && obj.error !== false)) {
    return null
  }

  if (typeof obj.app_id === 'string' || (typeof obj.draft_id === 'string' && typeof obj.bundle_sha256 === 'string')) {
    return obj
  }
  for (const key of ['draft', 'structuredContent', 'result'] as const) {
    if (obj[key] != null) {
      const nested = extractCandidate(obj[key], depth + 1)
      if (nested) return nested
    }
  }
  if (Array.isArray(obj.content)) {
    for (const part of obj.content) {
      const rec = asRecord(part)
      if (rec && typeof rec.text === 'string') {
        const nested = extractCandidate(rec.text, depth + 1)
        if (nested) return nested
      }
    }
  }
  return null
}

export function isAppDraftExpired(expiresAt?: string, nowMs = Date.now()): boolean {
  if (!expiresAt) return false
  const parsed = Date.parse(expiresAt)
  return Number.isFinite(parsed) && parsed <= nowMs
}

const strings = (value: unknown): string[] =>
  Array.isArray(value) ? value.filter((x): x is string => typeof x === 'string') : []

export function parseAppToolOutput(rawOutput: unknown): AppToolResult | null {
  const c = extractCandidate(rawOutput)
  if (!c) return null
  const title = typeof c.title === 'string' && c.title.trim() ? c.title.trim() : 'Untitled App'
  const kind: AppKind = c.kind === 'document' ? 'document' : 'artifact'
  if (typeof c.app_id === 'string') {
    return c.app_id ? { app: { id: c.app_id, title, kind } } : null
  }
  if (!c.draft_id || !c.bundle_sha256) return null
  return {
    draft: {
      draft_id: c.draft_id as string,
      bundle_sha256: c.bundle_sha256 as string,
      target_app_id: typeof c.target_app_id === 'string' && c.target_app_id ? c.target_app_id : undefined,
      title,
      kind,
      capabilities: strings(c.capabilities),
      allowed_tools: strings(c.allowed_tools),
      expires_at: typeof c.expires_at === 'string' ? c.expires_at : '',
    },
  }
}
