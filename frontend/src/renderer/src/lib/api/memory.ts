import { queryOptions } from '@tanstack/react-query'
import { get, post, put } from './client'
import type { MemoryDreamRunResponse, MemoryHorizon, MemoryIndexReport, MemoryStatus } from './types'
import { keys } from '@/lib/query/keys'

export const memoryQuery = queryOptions({
  queryKey: keys.memory,
  queryFn: () => get<MemoryStatus>('/v1/memory'),
})

export interface MemorySettingsInput {
  enabled?: boolean
  agent?: string
}

export function updateMemorySettings(input: MemorySettingsInput): Promise<MemoryStatus> {
  return put<MemoryStatus>('/v1/memory', input)
}

export function updateMemoryEnabled(enabled: boolean): Promise<MemoryStatus> {
  return updateMemorySettings({ enabled })
}

export function saveMemoryHorizon(name: string, content: string): Promise<MemoryHorizon> {
  return put<MemoryHorizon>(`/v1/memory/horizons/${encodeURIComponent(name)}`, { content })
}

export function reindexMemory(): Promise<MemoryIndexReport> {
  return post<MemoryIndexReport>('/v1/memory/reindex')
}

export function dreamMemory(): Promise<MemoryDreamRunResponse> {
  return post<MemoryDreamRunResponse>('/v1/memory/dream')
}
