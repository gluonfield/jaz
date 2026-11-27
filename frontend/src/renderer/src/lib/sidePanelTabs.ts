import type { FileReference } from '@shared/fileReader'
import type { PluginView, PresentationEvent, MCPAppEvent, MCPEntrypoint } from '@/lib/api/types'
import type { ArtifactInput } from '@/lib/artifacts'
import type { OpenedFile } from '@/lib/mcpAppFiles'

export type PanelView = PluginView
export type PanelPresentation = Exclude<PresentationEvent, { kind: 'artifact' }>
export const isPanelPresentation = (view?: PresentationEvent): view is PanelPresentation => Boolean(view && view.kind !== 'artifact')

export type SidePanelTab =
  | { id: string; kind: 'preview' | 'terminal' | 'side-chat' | 'canvas' }
  | { id: string; kind: 'diff'; repo?: string }
  | { id: string; kind: 'file'; file: FileReference | null }
  // An app tab shows a thread app, a file's viewer, or the call an inline view opened larger.
  | { id: string; kind: 'app'; app: Pick<MCPEntrypoint, 'server_id' | 'tool' | 'title' | 'icon'>; file?: OpenedFile; call?: Pick<MCPAppEvent, 'arguments' | 'result'> }
  // A view tab shows a plugin's view, drawn from a file in the thread's folder.
  | { id: string; kind: 'view'; view: PanelView }
  // An artifact tab shows an inline artifact larger, beside the chat.
  | { id: string; kind: 'artifact'; artifact: ArtifactInput }
  | { id: string; kind: 'published-app'; app: PublishedAppRef }
export type PublishedAppRef = { id: string; title: string }
type PublishedAppTab = Extract<SidePanelTab, { kind: 'published-app' }>
// NewSidePanelTab is what the new-tab menu opens: a built-in kind, a thread app or a published app's tab.
export type NewSidePanelTab = Exclude<SidePanelTab['kind'], 'app' | 'view' | 'canvas' | 'artifact' | 'published-app'> | MCPEntrypoint | PublishedAppTab
export type SidePanelMode = 'tabs' | 'overview'
export const OVERVIEW_PANEL_WIDTH = 300

export type SidePanelTabs = { tabs: SidePanelTab[]; activeId: string | null; dismissedViews?: string[] }
export type SidePanelTabAction =
  | { type: 'open'; tab: SidePanelTab; after?: string }
  | { type: 'select' | 'close'; id: string }
  | { type: 'restore'; tabs: SidePanelTab[] }
  | { type: 'reorder'; ids: string[] }

export function sidePanelTabs(state: SidePanelTabs, action: SidePanelTabAction): SidePanelTabs {
  if (action.type === 'restore') {
    const restored = action.tabs.filter((tab) => !state.dismissedViews?.includes(tab.id))
    const replacements = new Map(restored.map((tab) => [tab.id, tab]))
    return {
      ...state,
      tabs: [...state.tabs.map((tab) => replacements.get(tab.id) ?? tab), ...restored.filter((tab) => !state.tabs.some((current) => current.id === tab.id))],
      activeId: state.activeId ?? restored[0]?.id ?? null,
    }
  }
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
      ...state,
      ...(state.dismissedViews?.includes(action.tab.id) && { dismissedViews: state.dismissedViews.filter((id) => id !== action.tab.id) }),
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
    ...state,
    ...(state.tabs[index].kind === 'view' && { dismissedViews: [...(state.dismissedViews ?? []), action.id] }),
    tabs,
    activeId: state.activeId === action.id ? tabs[Math.min(index, tabs.length - 1)]?.id ?? null : state.activeId,
  }
}

// The same artifact opens the same tab, so reopening it selects that tab.
export function artifactTab(artifact: ArtifactInput): Extract<SidePanelTab, { kind: 'artifact' }> {
  let hash = 0
  for (let index = 0; index < artifact.code.length; index++) {
    hash = (hash * 31 + artifact.code.charCodeAt(index)) | 0
  }
  return { id: `artifact:${artifact.title}:${hash}`, kind: 'artifact', artifact }
}

export function publishedAppTab(app: PublishedAppRef): PublishedAppTab {
  return { id: `published-app:${app.id}`, kind: 'published-app', app: { id: app.id, title: app.title } }
}

export function pluginViewTab(view: PluginView): Extract<SidePanelTab, { kind: 'view' }> {
  return { id: pluginViewId(view), kind: 'view', view: view.reference ? { ...view, widget_code: '' } : view }
}

function pluginViewId(view: PluginView): string {
  return `view:${JSON.stringify([view.reference ?? '', view.file])}`
}

export function refreshPluginTabs(tabs: SidePanelTab[], views: PluginView[]): SidePanelTab[] {
  const byId = new Map(views.map((view) => [pluginViewId(view), view]))
  return tabs.map((tab) => {
    if (tab.kind !== 'view' || !tab.view.reference) return tab
    const view = byId.get(tab.id)
    return view && view !== tab.view ? { ...tab, view } : tab
  })
}
