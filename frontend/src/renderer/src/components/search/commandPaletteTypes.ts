import type { LucideIcon } from 'lucide-react'
import type { ThreadSearchResult } from '@/lib/api/types'

export type PaletteCommand = {
  id: string
  kind: 'command'
  title: string
  icon?: LucideIcon
  shortcut?: string
  run: () => void
}

export type PaletteThread = {
  id: string
  kind: 'thread'
  result: ThreadSearchResult
}

export type PaletteItem = PaletteCommand | PaletteThread
