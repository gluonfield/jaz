import type { FileReference } from '@shared/fileReader'
import type { MCPAppEvent, MCPEntrypoint } from '@/lib/api/types'
import type { OpenedFile } from '@/lib/mcpAppFiles'

export type SidePanelTab =
  | { id: string; kind: 'preview' | 'terminal' | 'side-chat' | 'diff' }
  | { id: string; kind: 'file'; file: FileReference | null }
  | { id: string; kind: 'app'; app: MCPEntrypoint; file?: OpenedFile; call?: Pick<MCPAppEvent, 'arguments' | 'result'> }
// NewSidePanelTab is what the new-tab menu opens: a built-in kind or a thread app.
export type NewSidePanelTab = Exclude<SidePanelTab['kind'], 'app'> | MCPEntrypoint
export type SidePanelMode = 'tabs' | 'overview'
export const OVERVIEW_PANEL_WIDTH = 300

export type SidePanelTabs = { tabs: SidePanelTab[]; activeId: string | null }
export type SidePanelTabAction =
  | { type: 'open'; tab: SidePanelTab; after?: string }
  | { type: 'select' | 'close'; id: string }
  | { type: 'reorder'; ids: string[] }

export function sidePanelTabs(state: SidePanelTabs, action: SidePanelTabAction): SidePanelTabs {
  if (action.type === 'reorder') {
    const order = new Map(action.ids.map((id, index) => [id, index]))
    return {
      ...state,
      tabs: state.tabs.toSorted((a, b) => (order.get(a.id) ?? state.tabs.length) - (order.get(b.id) ?? state.tabs.length)),
    }
  }
  if (action.type === 'open') {
    const tabs = action.tab.kind === 'file' && action.tab.file
      ? state.tabs.filter((tab) => tab.kind !== 'file' || tab.file)
      : state.tabs
    const existing = tabs.some((tab) => tab.id === action.tab.id)
    const after = tabs.findIndex((tab) => tab.id === action.after)
    return {
      tabs: existing ? tabs.map((tab) => tab.id === action.tab.id ? action.tab : tab) : tabs.toSpliced(after < 0 ? tabs.length : after + 1, 0, action.tab),
      activeId: action.tab.id,
    }
  }
  const index = state.tabs.findIndex((tab) => tab.id === action.id)
  if (index < 0) {
    return state
  }
  if (action.type === 'select') {
    return { ...state, activeId: action.id }
  }
  const tabs = state.tabs.filter((tab) => tab.id !== action.id)
  return {
    tabs,
    activeId: state.activeId === action.id ? tabs[Math.min(index, tabs.length - 1)]?.id ?? null : state.activeId,
  }
}
