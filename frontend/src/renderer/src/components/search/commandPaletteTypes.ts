import type { LucideIcon } from 'lucide-react'
import type { ConnectionSearchResult, MCPEntrypoint, ThreadSearchResult } from '@/lib/api/types'

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

export type PaletteConnectionResult = {
  id: string
  kind: 'connection'
  result: ConnectionSearchResult
}

export type PaletteResult = PaletteThread | PaletteConnectionResult

export type PaletteItem = PaletteCommand | PaletteResult

export type PaletteSection = {
  id: string
  label: string
  // A connection's section leads with its server's icon.
  app?: Pick<MCPEntrypoint, 'icon' | 'title'>
  items: PaletteResult[]
}
