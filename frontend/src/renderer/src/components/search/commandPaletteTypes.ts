import type { ComponentType } from 'react'
import type { ThreadSearchResult } from '@/lib/api/types'
import type { ThreadSearchRole } from '@/lib/api/search'

export type RoleMode = 'all' | 'user' | 'assistant'

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

export function roleModeRoles(mode: RoleMode): ThreadSearchRole[] {
  switch (mode) {
    case 'user':
      return ['user']
    case 'assistant':
      return ['assistant']
    default:
      return ['user', 'assistant']
  }
}

export function threadTitle(result: ThreadSearchResult): string {
  return result.thread_title || result.thread_slug || result.thread_id
}
