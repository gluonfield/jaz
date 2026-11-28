import { queryOptions } from '@tanstack/react-query'
import { ApiError } from '@/lib/api/response'
import { keys } from '@/lib/query/keys'
import { get as cloudGet, put as cloudPut } from './client'
import type { CloudUser } from './serverTypes'

export type InstructionScope = 'org' | 'space' | 'user' | 'bot'
export type InstructionKind = 'agents' | 'bots' | 'memory' | 'short_term' | 'long_term'

export interface CloudInstructionKey {
  org_id?: string
  scope: InstructionScope
  scope_id: string
  kind: InstructionKind
}

export interface CloudInstructionDocument {
  key: CloudInstructionKey
  body_md: string
  version: string
  updated_by?: string
  updated_at?: string
  read_only?: boolean
}

export const INSTRUCTION_SCOPE_KINDS: Record<InstructionScope, InstructionKind[]> = {
  org: ['agents'],
  space: ['agents'],
  user: ['agents', 'short_term', 'long_term'],
  bot: ['bots', 'memory'],
}

export const INSTRUCTION_KIND_FILENAME: Record<InstructionKind, string> = {
  agents: 'AGENTS.md',
  bots: 'BOTS.md',
  memory: 'MEMORY.md',
  short_term: 'SHORT_TERM',
  long_term: 'LONG_TERM',
}

export function canEditCloudInstruction(user: CloudUser | null | undefined, scope: InstructionScope): boolean {
  if (!user) return false
  const role = user.role.trim().toLowerCase()
  if (role === 'viewer') return false
  if (scope === 'org' || scope === 'bot') return role === 'admin'
  return true
}

type RawInstructionRecord = {
  org_id?: string
  scope?: InstructionScope
  scope_id?: string
  kind?: InstructionKind
  key?: Partial<CloudInstructionKey>
  body_md?: string
  content?: string
  version?: string
  updated_by?: string
  updated_at?: string
  read_only?: boolean
}

function normalizeInstruction(
  scope: InstructionScope,
  scopeId: string,
  kind: InstructionKind,
  raw: RawInstructionRecord,
): CloudInstructionDocument {
  return {
    key: {
      org_id: raw.org_id ?? raw.key?.org_id,
      scope: raw.scope ?? raw.key?.scope ?? scope,
      scope_id: raw.scope_id ?? raw.key?.scope_id ?? scopeId,
      kind: raw.kind ?? raw.key?.kind ?? kind,
    },
    body_md: raw.body_md ?? raw.content ?? '',
    version: raw.version ?? '',
    updated_by: raw.updated_by,
    updated_at: raw.updated_at,
    read_only: raw.read_only,
  }
}

export function extractInstructionConflictCurrent(
  error: unknown,
  scope: InstructionScope,
  scopeId: string,
  kind: InstructionKind,
): CloudInstructionDocument | null {
  if (!(error instanceof ApiError) || error.status !== 409 || !error.body || typeof error.body !== 'object') {
    return null
  }
  const current = (error.body as { current?: RawInstructionRecord }).current
  if (!current || typeof current !== 'object') {
    return null
  }
  return normalizeInstruction(scope, scopeId, kind, current)
}

export async function getInstruction(
  scope: InstructionScope,
  scopeId: string,
  kind: InstructionKind,
  signal?: AbortSignal,
): Promise<CloudInstructionDocument> {
  try {
    const raw = await cloudGet<RawInstructionRecord>(
      `/v1/instructions/${encodeURIComponent(scope)}/${encodeURIComponent(scopeId)}/${encodeURIComponent(kind)}`,
      { signal },
    )
    return normalizeInstruction(scope, scopeId, kind, raw)
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) {
      return normalizeInstruction(scope, scopeId, kind, { body_md: '', version: '' })
    }
    throw error
  }
}

export async function saveInstruction(
  scope: InstructionScope,
  scopeId: string,
  kind: InstructionKind,
  input: { body_md: string; expected_version: string },
): Promise<CloudInstructionDocument> {
  const raw = await cloudPut<RawInstructionRecord>(
    `/v1/instructions/${encodeURIComponent(scope)}/${encodeURIComponent(scopeId)}/${encodeURIComponent(kind)}`,
    {
      body_md: input.body_md,
      expected_version: input.expected_version,
    },
  )
  return normalizeInstruction(scope, scopeId, kind, raw)
}

export const cloudInstructionQuery = (scope: InstructionScope, scopeId: string, kind: InstructionKind) =>
  queryOptions({
    queryKey: keys.cloudInstruction(scope, scopeId, kind),
    queryFn: ({ signal }) => getInstruction(scope, scopeId, kind, signal),
    enabled: Boolean(scopeId),
  })
