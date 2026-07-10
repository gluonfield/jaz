import { X } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { FileDropScope } from '@/components/ui/FileDrop'
import { IconButton } from '@/components/ui/IconButton'
import type { Attachment, ChatMessage, SessionEvent } from '@/lib/api/types'
import { contextInputs, type SendMessageOptions } from '@/lib/sendMessage'
import { ComposerCard } from './Composer'
import { SidePanelShell } from './SidePanelShell'
import { Transcript } from './Transcript'
import {
  latestSideChatID,
  latestSideChatText,
  liveSideChatSeq,
  sideChatEvents,
  sideChatHasEvents,
  sideChatTranscript,
  sideChatUserMessage,
} from './sideChatTranscript'

export const SIDE_CHAT_PANEL_WIDTH = 520

type LiveSideChatTurn = {
  sideChatID: string
  baselineMessageCount: number
  message: ChatMessage
}

type PendingSideChatTurn = {
  sideChatID: string
  baselineEventCount: number
}

export function SideChatPanel({
  sessionId,
  events,
  visible,
  onSend,
  onUploadAttachment,
  onClose,
  fileRoot,
}: {
  sessionId: string
  events: SessionEvent[]
  visible: boolean
  onSend: (sideChatID: string, message: string, options?: SendMessageOptions) => Promise<void>
  onUploadAttachment?: (file: File) => Promise<Attachment>
  onClose: () => void
  fileRoot?: string
}) {
  const [sideChatID, setSideChatID] = useState(() => latestSideChatID(events) || newSideChatID())
  const [pendingTurn, setPendingTurn] = useState<PendingSideChatTurn | null>(null)
  const [live, setLive] = useState<LiveSideChatTurn | null>(null)
  const [error, setError] = useState('')
  const scrollRef = useRef<HTMLDivElement>(null)
  const sideEvents = useMemo(() => sideChatEvents(events, sideChatID), [events, sideChatID])
  const transcript = useMemo(() => sideChatTranscript(sideEvents), [sideEvents])
  const liveVisible = Boolean(
    live &&
      live.sideChatID === sideChatID &&
      transcript.messages.length <= live.baselineMessageCount,
  )
  const transcriptMessages = useMemo(
    () =>
      liveVisible && live
        ? [...transcript.messages, live.message]
        : transcript.messages,
    [live, liveVisible, transcript.messages],
  )
  const pending = Boolean(pendingTurn)
  const pendingActive = Boolean(pendingTurn && pendingTurn.sideChatID === sideChatID)
  const pendingOutputStarted = Boolean(
    pendingTurn &&
      pendingTurn.sideChatID === sideChatID &&
      sideEvents.slice(pendingTurn.baselineEventCount).some((event) => event.side_chat?.role !== 'user'),
  )
  const latestTranscriptText = latestSideChatText(transcriptMessages, transcript.events)

  useEffect(() => {
    if (live) return
    const latest = latestSideChatID(events)
    if (!latest) return
    setSideChatID((current) => {
      if (current === latest) return current
      return sideChatHasEvents(events, current) ? current : latest
    })
  }, [events, live])

  useEffect(() => {
    if (!live || live.sideChatID !== sideChatID) return
    if (transcript.messages.length > live.baselineMessageCount) setLive(null)
  }, [live, sideChatID, transcript.messages.length])

  useEffect(() => {
    if (!visible) return
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight })
  }, [visible, transcriptMessages.length, transcript.events.length, latestTranscriptText, pendingActive, pendingOutputStarted])

  useEffect(() => {
    if (transcriptMessages.length > 0) setError('')
  }, [transcriptMessages.length])

  const close = () => {
    onClose()
    setError('')
  }

  const submit = (message: string, options?: SendMessageOptions) => {
    if (pendingTurn) return
    setError('')
    setPendingTurn({
      sideChatID,
      baselineEventCount: sideEvents.length,
    })
    setLive({
      sideChatID,
      baselineMessageCount: transcript.messages.length,
      message: sideChatUserMessage(
        message,
        contextInputs(options?.contexts ?? []),
        options?.attachments ?? [],
        new Date().toISOString(),
        liveSideChatSeq(sideEvents),
      ),
    })
    void onSend(sideChatID, message, options)
      .catch((err: Error) => {
        setError(err.message || 'Side chat failed.')
      })
      .finally(() => {
        setPendingTurn(null)
      })
  }

  return (
    <FileDropScope className="h-full">
      <SidePanelShell width={SIDE_CHAT_PANEL_WIDTH}>
        <div className="flex h-11 shrink-0 items-center justify-between border-b border-border px-3">
          <div className="min-w-0 text-sm font-medium text-ink">Side chat</div>
          <div className="flex items-center gap-1">
            <IconButton size="sm" aria-label="Close side chat" title="Close side chat" onClick={close}>
              <X size={15} />
            </IconButton>
          </div>
        </div>
        <div ref={scrollRef} className="scrollbar-quiet min-h-0 flex-1 overflow-y-auto bg-bg px-4 py-4">
          <div className="flex min-h-full flex-col justify-end">
            <Transcript
              messages={transcriptMessages}
              events={transcript.events}
              sessionId={sideChatID}
              attachmentSessionId={sessionId}
              working={pendingActive}
              tail={
                pendingActive && !pendingOutputStarted ? (
                  <p className="animate-pulse text-sm text-ink-3">Thinking…</p>
                ) : null
              }
            />
          </div>
        </div>
        {error ? <div className="px-3 pb-2 text-[12px] text-danger">{error}</div> : null}
        <div className="shrink-0 border-t border-border bg-bg p-2">
          <ComposerCard
            streaming={pending}
            disabled={pending}
            placeholder="Ask in side chat"
            draftStorageKey={`side-chat:${sideChatID}`}
            fileRoot={fileRoot}
            attachmentSessionId={sessionId}
            onSend={submit}
            onUploadAttachment={onUploadAttachment}
            onTextChange={() => {
              if (error) setError('')
            }}
          />
        </div>
      </SidePanelShell>
    </FileDropScope>
  )
}

function newSideChatID(): string {
  const raw = typeof crypto !== 'undefined' && crypto.randomUUID ? crypto.randomUUID() : String(Date.now())
  return `side_${raw.replaceAll('-', '')}`
}
