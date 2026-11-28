import { queryOptions } from '@tanstack/react-query'
import { keys } from '../query/keys'
import { appsQuery, type JazApp } from './apps'
import { del, get, patch, post } from './client'
import { activeRunStatus } from './loops'
import type { Loop, LoopRunStatus } from './types'

// A board is an app of kind board: it is listed, renamed, shared and deleted
// as an app, and holds other apps as tiles.
export const boardsQuery = appsQuery({ kind: 'board' })

export interface BoardTile {
  app: JazApp
  x: number
  y: number
  w: number
  h: number
  // The loop that republishes the app on every run; absent on a pinned app.
  loop?: {
    id: string
    name: string
    status: Loop['status']
    last_run_status?: LoopRunStatus
    last_run_at?: string
  }
  last_error?: string
}

export interface BoardView {
  // Zooms every tile's document.
  font_scale: number
  grid_cols: number
  row_height: number
  tiles: BoardTile[]
}

const boardPath = (id: string) => `/v1/apps/${encodeURIComponent(id)}/board`

export const boardViewQuery = (id: string) =>
  queryOptions({
    queryKey: keys.appBoard(id),
    queryFn: () => get<BoardView>(boardPath(id)),
    // A publish shows up as a tile app's new version; poll tighter while any
    // tile's loop is mid-run.
    refetchInterval: (query) =>
      query.state.data?.tiles.some((tile) => activeRunStatus(tile.loop?.last_run_status)) ? 3_000 : 15_000,
  })

export async function createBoard(title: string): Promise<JazApp> {
  return (await post<{ app: JazApp }>('/v1/apps', { kind: 'board', title })).app
}

export interface BoardLayoutEntry {
  app_id: string
  x: number
  y: number
  w: number
  h: number
}

export function patchBoard(id: string, input: { font_scale?: number; layout?: BoardLayoutEntry[] }): Promise<{ ok: boolean }> {
  return patch<{ ok: boolean }>(boardPath(id), input)
}

// Pins apps as they are, and puts loops on the board so each run republishes
// the loop's app there. Loops keep the other boards they are on.
export function addBoardTiles(id: string, input: { app_ids?: string[]; loop_ids?: string[] }): Promise<{ ok: boolean }> {
  return post<{ ok: boolean }>(`${boardPath(id)}/tiles`, input)
}

export function removeBoardTile(id: string, appId: string): Promise<{ ok: boolean }> {
  return del<{ ok: boolean }>(`${boardPath(id)}/tiles/${encodeURIComponent(appId)}`)
}

// What a tile measured while showing a loop's app: dead space, overflow,
// clipped elements and broken images.
export interface TileLayoutReport {
  dead_space_pct: number
  overflow_px: number
  clipped: number
  img_errors: number
}

// The loop reads this feedback in its next run's prompt.
export function reportTile(appId: string, feedback: { error: string } | { layout: TileLayoutReport }): Promise<{ ok: boolean }> {
  return post<{ ok: boolean }>(`/v1/apps/${encodeURIComponent(appId)}/tile-feedback`, feedback)
}
