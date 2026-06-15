import type { ComponentType } from 'react'
import type { ThreadSearchResult } from '@/lib/api/types'

export type PaletteCommand = {
  id: string
  kind: 'command'
  title: string
  detail: string
  Icon: ComponentType<{ size?: number; className?: string }>
  shortcut?: string
  run: () => void
}

export type PaletteItem =
  | PaletteCommand
  | {
      id: string
      kind: 'thread'
      result: ThreadSearchResult
    }

export function threadTitle(result: ThreadSearchResult): string {
  return result.thread_title || result.thread_slug || result.thread_id
}
