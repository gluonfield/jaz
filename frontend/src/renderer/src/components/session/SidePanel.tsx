import type { ReactNode } from 'react'
import type { Attachment, QueuedAction, Session, SessionEvent } from '@/lib/api/types'
import type { BrowserAnnotation } from '@/lib/messageContext'
import type { ProviderSubagentView } from '@/lib/providerSubagents'
import { MCPAppFrame } from '@/components/apps/MCPAppFrame'
import { BrowserPanelSlot } from '@/components/browser/BrowserWorkspace'
import type { SendMessageHandler, SendMessageOptions } from '@/lib/sendMessage'
import type { SpawnedThreadView } from '@/lib/spawnedThreads'
import type { TaskSurface } from '@/lib/taskSurface'
import { CodeDiffPanel } from '@/components/session/CodeDiffPanel'
import { FileReaderPanel } from '@/components/session/FileReaderPanel'
import { OverviewPanel } from '@/components/session/OverviewPanel'
import { SideChatPanel } from '@/components/session/SideChatPanel'
import { TerminalPanel } from '@/components/session/TerminalPanel'
import { SidePanelControl } from '@/components/session/SidePanelControl'
import { SidePanelTabMenu, SidePanelTabs } from '@/components/session/SidePanelTabs'
import { useIsMobile } from '@/lib/hooks/useIsMobile'
import type { useSidePanelState } from '@/components/session/SidePanelState'

export function SidePanel({
  session, panel, progress, subagents, spawnedThreads, working,
  sideChatAvailable, sideChatEvents, onAddBrowserAnnotation, onUploadAttachment,
  onSend, onQueuePrompt, onQueueAction, onSendSideChat, overview, overviewLabel,
}: {
  session: Session
  panel: ReturnType<typeof useSidePanelState>
  progress?: TaskSurface
  subagents: ProviderSubagentView[]
  spawnedThreads: SpawnedThreadView[]
  working: boolean
  sideChatAvailable: boolean
  sideChatEvents: SessionEvent[]
  onAddBrowserAnnotation?: (annotation: BrowserAnnotation, screenshot?: Attachment) => void
  onUploadAttachment?: (file: File) => Promise<Attachment>
  onSend: SendMessageHandler
  onQueuePrompt: SendMessageHandler
  onQueueAction: (action: QueuedAction, label: string) => Promise<void>
  onSendSideChat: (sideChatID: string, message: string, options?: SendMessageOptions) => Promise<void>
  overview?: ReactNode
  overviewLabel?: string
}) {
  const isMobile = useIsMobile()
  return (
    <div className={`flex h-full min-h-0 flex-col ${panel.mode === 'tabs' ? 'bg-surface' : ''}`} data-thread-find-shortcuts="off">
      {/* Desktop: the panel's own header carries its tabs and the controls that
          close it; phones keep both in the titlebar. */}
      {!isMobile ? (
        <div className="flex h-10 shrink-0 items-center justify-end gap-1 px-1.5">
          {panel.mode === 'tabs' ? <SidePanelTabs
            tabs={panel.tabs}
            activeId={panel.activeTab?.id}
            sideChatAvailable={sideChatAvailable}
            onSelect={panel.selectTab}
            onReorder={panel.reorderTabs}
            onClose={panel.closeTab}
            onAdd={panel.addTab}
            onDuplicate={panel.duplicateTab}
          /> : null}
          <SidePanelControl open={panel.open} mode={panel.mode} overviewLabel={overviewLabel} onToggle={panel.toggleMode} />
        </div>
      ) : null}
      {panel.open && panel.mode === 'overview' ? <div className="min-h-0 flex-1">{overview ?? (
        <OverviewPanel
          session={session}
          progress={progress}
          subagents={subagents}
          spawnedThreads={spawnedThreads}
          working={working}
          onSend={onSend}
          onQueuePrompt={onQueuePrompt}
          onQueueAction={onQueueAction}
        />
      )}</div> : null}
      <div hidden={panel.mode !== 'tabs'} className="relative flex min-h-0 flex-1 flex-col overflow-hidden">
        {!panel.tabs.length ? <SidePanelTabMenu empty sideChatAvailable={sideChatAvailable} onAdd={panel.addTab} /> : null}
        {panel.tabs.map((tab) => {
          const active = panel.activeTab?.id === tab.id
          const visible = panel.open && panel.mode === 'tabs' && active
          return (
            <div
              key={tab.id}
              id={`panel-body-${tab.id}`}
              role="tabpanel"
              aria-labelledby={`panel-tab-${tab.id}`}
              hidden={!active}
              inert={!visible}
              className="min-h-0 flex-1"
            >
              {tab.kind === 'preview' ? (
                <BrowserPanelSlot
                  sessionId={tab.id}
                  visible={visible}
                  embedded
                  onAddBrowserAnnotation={onAddBrowserAnnotation}
                  onUploadAttachment={onUploadAttachment}
                />
              ) : tab.kind === 'terminal' ? (
                <TerminalPanel session={session} visible={visible} />
              ) : tab.kind === 'file' ? (
                <FileReaderPanel sessionId={session.id} fileRef={tab.file} visible={visible} onOpenFile={panel.openFile} onClose={panel.close} />
              ) : tab.kind === 'diff' ? (
                <CodeDiffPanel sessionId={session.id} visible={visible} />
              ) : tab.kind === 'app' ? (
                <MCPAppFrame app={tab.app} file={tab.file} active={visible} />
              ) : (
                <SideChatPanel
                  sessionId={session.id}
                  events={sideChatEvents}
                  visible={visible}
                  fileRoot={session.runtime_ref?.cwd}
                  onUploadAttachment={onUploadAttachment}
                  onSend={onSendSideChat}
                />
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}
