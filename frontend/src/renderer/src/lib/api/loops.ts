import { queryOptions } from '@tanstack/react-query'
import { keys } from '../query/keys'
import { del, get, patch, post } from './client'
import type { Loop, LoopRun } from './types'

export interface LoopInput {
  name?: string
  prompt: string
  schedule: { kind: string; expr: string; timezone: string }
  status?: string
  runtime?: string
  acp_agent?: string
  reasoning_effort?: string
  directory?: string
}

const activeRunStatus = (status?: string) => status === 'starting' || status === 'running'

export const loopsQuery = queryOptions({
  queryKey: keys.loops,
  queryFn: async () => {
    const data = await get<{ loops: Loop[] | null }>('/v1/loops')
    return data.loops ?? []
  },
  // Tighten the poll while a run is in flight so status surfaces live.
  refetchInterval: (query) =>
    query.state.data?.some((loop) => activeRunStatus(loop.last_run_status)) ? 3_000 : 20_000,
})

export interface LoopDetail {
  loop: Loop
  runs: LoopRun[]
}

export const loopDetailQuery = (id: string) =>
  queryOptions({
    queryKey: keys.loopDetail(id),
    queryFn: async () => {
      const data = await get<{ loop: Loop; runs: LoopRun[] | null }>(`/v1/loops/${id}`)
      return { loop: data.loop, runs: data.runs ?? [] } satisfies LoopDetail
    },
    refetchInterval: (query) =>
      activeRunStatus(query.state.data?.runs[0]?.status) ? 2_000 : false,
  })

export function createLoop(input: LoopInput): Promise<Loop> {
  return post<Loop>('/v1/loops', input)
}

export function updateLoop(id: string, input: Partial<LoopInput>): Promise<Loop> {
  return patch<Loop>(`/v1/loops/${id}`, input)
}

export function deleteLoop(id: string): Promise<{ ok: boolean }> {
  return del<{ ok: boolean }>(`/v1/loops/${id}`)
}

export function runLoopNow(id: string): Promise<LoopRun> {
  return post<LoopRun>(`/v1/loops/${id}/run`)
}
