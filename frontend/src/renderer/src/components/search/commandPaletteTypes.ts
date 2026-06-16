import type { ThreadSearchResult } from '@/lib/api/types'

export type PaletteCommand = {
  id: string
  kind: 'command'
  title: string
  detail: string
  shortcut?: string
  run: () => void
}

export type PaletteThread = {
  id: string
  kind: 'thread'
  result: ThreadSearchResult
}

export type PaletteItem = PaletteCommand | PaletteThread

export function threadTitle(result: ThreadSearchResult): string {
  return result.thread_title || result.thread_slug || result.thread_id
}
