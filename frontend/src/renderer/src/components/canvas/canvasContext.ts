import { createContext, useContext } from 'react'
import type { CanvasItem, CanvasPatch, CanvasView } from '@/lib/api/canvas'
import type { Rect } from './board'

export interface CanvasBoard {
  sessionId: string
  view: CanvasView
  theme: string
  focused: string | null
  setFocused: (id: string | null) => void
  // When the agent last changed each item, so it can flash.
  flash: Record<string, number>
  editing: string | null
  setEditing: (id: string | null) => void
  dropTarget: string | null
  connectSource: string | null
  commit: (change: CanvasPatch) => void
  // Puts a new item on the board, with the connectors that come with it.
  add: (item: CanvasItem & Rect, edges?: Partial<CanvasItem>[]) => void
  // Copies the selection beside itself.
  duplicate: () => void
  prompt: (text: string) => void
}

export const CanvasContext = createContext<CanvasBoard | null>(null)

export const useCanvas = () => useContext(CanvasContext)!
