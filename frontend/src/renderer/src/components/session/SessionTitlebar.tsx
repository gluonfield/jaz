import { type ReactNode, useMemo } from 'react'
import { SidePanelControl } from '@/components/session/SidePanelControl'
import type { useSidePanelState } from '@/components/session/SidePanelState'
import { SidePanelTabs } from '@/components/session/SidePanelTabs'
import { TokenStats } from '@/components/session/TokenStats'
import { RuntimeBadge } from '@/components/sidebar/RuntimeBadge'
import type { Session } from '@/lib/api/types'
import { useTitlebarActions, useTitlebarSlot } from '@/lib/titlebar'

export function SessionTitlebar({ session, isMobile, panel, sideChatAvailable, header, overviewLabel }: {
  session: Session
  isMobile: boolean
  panel: ReturnType<typeof useSidePanelState>
  sideChatAvailable: boolean
  header?: ReactNode
  overviewLabel?: string
}) {
  const { open, mode, tabs, activeTab, width, selectTab, reorderTabs, closeTab, addTab, duplicateTab, toggleMode } = panel
  const activeId = activeTab?.id
  const slot = useMemo(() => header ?? <>
    <RuntimeBadge session={session} truncate />
    <TokenStats session={session} />
  </>, [header, session])
  useTitlebarSlot(slot)
  const actions = useMemo(() => (
    <div className="flex min-w-0 items-center gap-1" style={{ width: open && mode === 'tabs' ? isMobile ? '100%' : `calc(${width}px - 1rem)` : undefined }}>
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
      <SidePanelControl open={open} mode={mode} overviewLabel={overviewLabel} onToggle={toggleMode} />
    </div>
  ), [open, mode, isMobile, width, tabs, activeId, sideChatAvailable, overviewLabel, selectTab, reorderTabs, closeTab, addTab, duplicateTab, toggleMode])
  useTitlebarActions(actions)
  return null
}
