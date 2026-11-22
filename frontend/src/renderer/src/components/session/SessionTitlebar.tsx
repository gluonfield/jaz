import { type ReactNode, useMemo, useState } from 'react'
import { PanelsTopLeft } from 'lucide-react'
import { Popover } from '@/components/ui/Popover'
import { SidePanelControl } from '@/components/session/SidePanelControl'
import type { useSidePanelState } from '@/components/session/SidePanelState'
import { SidePanelTabs } from '@/components/session/SidePanelTabs'
import { TokenStats } from '@/components/session/TokenStats'
import { CloudThreadControls } from '@/components/session/CloudThreadControls'
import { RuntimeBadge } from '@/components/sidebar/RuntimeBadge'
import type { PluginView, Session } from '@/lib/api/types'
import { useTitlebarActions, useTitlebarSlot } from '@/lib/titlebar'

export function SessionTitlebar({ session, isMobile, panel, sideChatAvailable, header, overviewLabel, pluginViews }: {
  session: Session
  isMobile: boolean
  panel: ReturnType<typeof useSidePanelState>
  sideChatAvailable: boolean
  header?: ReactNode
  overviewLabel?: string
  pluginViews?: PluginView[]
}) {
  const { open, mode, tabs, activeTab, selectTab, reorderTabs, closeTab, addTab, duplicateTab, toggleMode } = panel
  const activeId = activeTab?.id
  const [viewsOpen, setViewsOpen] = useState(false)
  const slot = useMemo(() => header ?? <>
    <RuntimeBadge session={session} truncate />
    <TokenStats session={session} />
    <CloudThreadControls session={session} />
  </>, [header, session])
  useTitlebarSlot(slot)
  // On desktop the open panel's own header carries its tabs and controls.
  const actions = useMemo(() => {
    const views = pluginViews?.length ? <Popover open={viewsOpen} onClose={() => setViewsOpen(false)} placement="below" align="end" trigger={
      <button type="button" aria-label="Plugin views" title="Plugin views" aria-expanded={viewsOpen} onClick={() => setViewsOpen((value) => !value)} className="grid h-7 w-7 shrink-0 place-items-center rounded-lg text-ink-2 hover:bg-surface hover:text-ink"><PanelsTopLeft size={14} /></button>
    }><div role="menu">{pluginViews.map((view) => <button key={`${view.reference}:${view.file}`} type="button" role="menuitem" className="block w-full rounded-lg px-3 py-2 text-left text-sm text-ink hover:bg-surface-2" onClick={() => { panel.openView({ kind: 'plugin-view', view }); setViewsOpen(false) }}>{view.title}</button>)}</div></Popover> : null
    const control = <SidePanelControl open={open} mode={mode} overviewLabel={overviewLabel} onToggle={toggleMode} />
    if (!isMobile) {
      return <div className="flex items-center gap-1">{views}{open ? null : control}</div>
    }
    return (
      <div className="flex min-w-0 items-center gap-1" style={{ width: open && mode === 'tabs' ? '100%' : undefined }}>
        {open && mode === 'tabs' ? <SidePanelTabs
          tabs={tabs}
          activeId={activeId}
          sideChatAvailable={sideChatAvailable}
          onSelect={selectTab}
          onReorder={reorderTabs}
          onClose={closeTab}
          onAdd={addTab}
          onDuplicate={duplicateTab}
        /> : null}
        {views}
        {control}
      </div>
    )
  }, [pluginViews, viewsOpen, panel.openView, open, mode, isMobile, tabs, activeId, sideChatAvailable, overviewLabel, selectTab, reorderTabs, closeTab, addTab, duplicateTab, toggleMode])
  useTitlebarActions(actions)
  return null
}
