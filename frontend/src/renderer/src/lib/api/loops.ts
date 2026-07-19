import { queryOptions } from '@tanstack/react-query'
import { telemetry } from '@/lib/telemetry'
import { keys } from '../query/keys'
import { del, get, patch, post } from './client'
import type { Loop, LoopRun, LoopRunStatus } from './types'

export interface LoopInput {
  name?: string
  prompt: string
  schedule: { kind: string; expr: string; timezone: string }
  status?: string
  runtime?: string
  acp_agent?: string
  // Overrides of the Settings > Agents defaults; '' follows settings, "none" clears it.
  model_provider?: string
  model?: string
  reasoning_effort?: string
  directory?: string
  // Boards the loop's widget is assigned to; assignment is what enables the
  // widget (there is no separate toggle).
  board_ids?: string[]
}

export const activeRunStatus = (status?: LoopRunStatus) => status === 'starting' || status === 'running'

// Shared loop status → indicator model: loop-level dots, pills, and the
// legend key off these tones. Per-run dots (ok/cancelled/skipped) stay local.
export type LoopTone = 'failed' | 'running' | 'paused' | 'active'

export const loopTone = (lastRunStatus?: LoopRunStatus, status?: Loop['status']): LoopTone =>
  lastRunStatus === 'error'
    ? 'failed'
    : activeRunStatus(lastRunStatus)
      ? 'running'
      : status === 'paused'
        ? 'paused'
        : 'active'

export const TONE_DOT: Record<LoopTone, string> = {
  failed: 'bg-danger',
  running: 'bg-running animate-pulse',
  paused: 'bg-ink-3/40',
  active: 'bg-primary',
}

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
  boardIds: string[]
}

export const loopDetailQuery = (id: string) =>
  queryOptions({
    queryKey: keys.loopDetail(id),
    queryFn: async () => {
      const data = await get<{ loop: Loop; runs: LoopRun[] | null; board_ids?: string[] }>(
        `/v1/loops/${id}`,
      )
      return {
        loop: data.loop,
        runs: data.runs ?? [],
        boardIds: data.board_ids ?? [],
      } satisfies LoopDetail
    },
    refetchInterval: (query) =>
      activeRunStatus(query.state.data?.runs[0]?.status) ? 2_000 : false,
  })

export async function createLoop(input: LoopInput, options: { runAfterCreate?: boolean } = {}): Promise<Loop> {
  const loop = await post<Loop>('/v1/loops', input)
  telemetry.loopCreated({
    runAfterCreate: Boolean(options.runAfterCreate),
    scheduleKind: input.schedule.kind,
    status: input.status ?? 'active',
    hasDirectory: Boolean(input.directory),
    boardCount: input.board_ids?.length ?? 0,
    hasModelOverride: Boolean(input.model),
    hasProviderOverride: Boolean(input.model_provider),
    hasReasoningEffort: Boolean(input.reasoning_effort),
  })
  return loop
}

export function updateLoop(id: string, input: Partial<LoopInput>): Promise<Loop> {
  return patch<Loop>(`/v1/loops/${id}`, input)
}

export function deleteLoop(id: string): Promise<{ ok: boolean }> {
  return del<{ ok: boolean }>(`/v1/loops/${id}`)
}

export async function runLoopNow(id: string): Promise<LoopRun> {
  const run = await post<LoopRun>(`/v1/loops/${id}/run`)
  telemetry.loopRunStarted()
  return run
}
