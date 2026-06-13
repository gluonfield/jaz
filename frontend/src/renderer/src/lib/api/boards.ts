import { queryOptions } from '@tanstack/react-query'
import { keys } from '../query/keys'
import { apiFetch, del, get, patch, post } from './client'
import type { Board, BoardItem } from './types'

const activeRunStatus = (status?: string) => status === 'starting' || status === 'running'

export const boardsQuery = queryOptions({
  queryKey: keys.boards,
  queryFn: async () => {
    const data = await get<{ boards: Board[] | null }>('/v1/boards')
    return data.boards ?? []
  },
})

export interface BoardDetail {
  board: Board
  items: BoardItem[]
}

export const boardDetailQuery = (id: string) =>
  queryOptions({
    queryKey: keys.boardDetail(id),
    queryFn: async () => {
      const data = await get<{ board: Board; items: BoardItem[] | null }>(`/v1/boards/${id}`)
      return { board: data.board, items: data.items ?? [] } satisfies BoardDetail
    },
    // Tiles refresh when a publish bumps current_version; poll tighter while
    // any backing loop is mid-run.
    refetchInterval: (query) =>
      query.state.data?.items.some((item) => activeRunStatus(item.loop_last_run_status))
        ? 3_000
        : 15_000,
  })

export function createBoard(name: string): Promise<Board> {
  return post<Board>('/v1/boards', { name })
}

export interface BoardLayoutEntry {
  widget_id: string
  x: number
  y: number
  w: number
  h: number
}

export function patchBoard(
  id: string,
  input: { name?: string; font_scale?: number; layout?: BoardLayoutEntry[] },
): Promise<Board> {
  return patch<Board>(`/v1/boards/${id}`, input)
}

export function deleteBoard(id: string): Promise<{ ok: boolean }> {
  return del<{ ok: boolean }>(`/v1/boards/${id}`)
}

export function removeWidgetFromBoard(boardId: string, widgetId: string): Promise<{ ok: boolean }> {
  return del<{ ok: boolean }>(`/v1/boards/${boardId}/widgets/${widgetId}`)
}

// Additive: assigns loops to this board without touching their other boards.
export function assignLoopsToBoard(boardId: string, loopIds: string[]): Promise<{ ok: boolean }> {
  return post<{ ok: boolean }>(`/v1/boards/${boardId}/loops`, { loop_ids: loopIds })
}

export function reportWidgetError(widgetId: string, message: string): Promise<{ ok: boolean }> {
  return post<{ ok: boolean }>(`/v1/widgets/${widgetId}/errors`, { message })
}

// Bridge-measured layout telemetry (dead space, overflow, clipped elements,
// broken images); the backend surfaces problems in the loop's next-run prompt.
export interface WidgetLayoutReport {
  dead_space_pct: number
  overflow_px: number
  clipped: number
  img_errors: number
}

export function reportWidgetLayout(
  widgetId: string,
  layout: WidgetLayoutReport,
): Promise<{ ok: boolean }> {
  return post<{ ok: boolean }>(`/v1/widgets/${widgetId}/layout`, layout)
}

export function widgetContentPath(
  widgetId: string,
  version: number,
  theme: string,
  zoom = 1,
): string {
  const params = new URLSearchParams({
    version: String(version),
    theme,
    inline_assets: '1',
  })
  if (zoom !== 1) params.set('zoom', String(zoom))
  return `/v1/widgets/${encodeURIComponent(widgetId)}/content?${params.toString()}`
}

export async function fetchWidgetContent(
  widgetId: string,
  version: number,
  theme: string,
  zoom = 1,
  signal?: AbortSignal,
): Promise<string> {
  const res = await apiFetch(widgetContentPath(widgetId, version, theme, zoom), { signal })
  if (!res.ok) throw new Error(`Widget content failed with ${res.status}`)
  return res.text()
}
