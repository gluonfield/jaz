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
import { SidePanelTabMenu, SidePanelTabs } from '@/components/session/SidePanelTabs'
import { useIsMobile } from '@/lib/hooks/useIsMobile'
import type { useSidePanelState } from '@/components/session/SidePanelState'

export function SidePanel({
  session, panel, progress, subagents, spawnedThreads, working,
  sideChatAvailable, sideChatEvents, onAddBrowserAnnotation, onUploadAttachment,
  onSend, onQueuePrompt, onQueueAction, onSendSideChat, overview,
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
}) {
  const isMobile = useIsMobile()
  return (
    <>
      {panel.open && panel.mode === 'overview' ? overview ?? (
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
      ) : null}
      <div hidden={panel.mode !== 'tabs'} className="relative flex h-full min-h-0 flex-col overflow-hidden bg-surface" data-thread-find-shortcuts="off">
        {/* Desktop: the tabs head the panel's own card; phones keep them in the titlebar. */}
        {!isMobile ? (
          <div className="flex h-10 shrink-0 items-center px-1.5">
            <SidePanelTabs
              tabs={panel.tabs}
              activeId={panel.activeTab?.id}
              sideChatAvailable={sideChatAvailable}
              onSelect={panel.selectTab}
              onReorder={panel.reorderTabs}
              onClose={panel.closeTab}
              onAdd={panel.addTab}
              onDuplicate={panel.duplicateTab}
            />
          </div>
        ) : null}
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
                <FileReaderPanel sessionId={session.id} fileRef={tab.file} visible={visible} onOpenFile={panel.openFile} />
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
    </>
  )
}
