import { queryOptions } from '@tanstack/react-query'
import { keys } from '../query/keys'
import { apiBaseUrl, del, get, patch, post } from './client'
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

// Bridge-measured layout telemetry (dead space, overflow, clipped elements);
// the backend surfaces problems in the loop's next-run prompt.
export function reportWidgetLayout(
  widgetId: string,
  layout: { dead_space_pct: number; overflow_px: number; clipped: number },
): Promise<{ ok: boolean }> {
  return post<{ ok: boolean }>(`/v1/widgets/${widgetId}/layout`, layout)
}

// Version in the URL makes a publish naturally reload the iframe; theme and
// zoom in the URL paint the first frame right (live changes go over the bridge).
export function widgetContentUrl(
  widgetId: string,
  version: number,
  theme: string,
  zoom = 1,
): string {
  const zoomParam = zoom !== 1 ? `&zoom=${zoom}` : ''
  return `${apiBaseUrl()}/v1/widgets/${widgetId}/content?version=${version}&theme=${theme}${zoomParam}`
}
