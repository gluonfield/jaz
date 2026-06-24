import { X } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { FileDropScope } from '@/components/ui/FileDrop'
import { IconButton } from '@/components/ui/IconButton'
import type { Attachment, SessionEvent } from '@/lib/api/types'
import type { ComposerContext, MessageContextInput, SendMessageOptions } from '@/lib/sendMessage'
import { AssistantBubble, type BubbleAttachment, UserBubble } from './Bubble'
import { ComposerCard } from './Composer'
import { SessionErrorNotice } from './SessionErrorNotice'
import { SidePanelShell } from './SidePanelShell'
import { ThinkingBlock } from './ThinkingBlock'
import { ToolStatusLine } from './ToolDisclosure'

export const SIDE_CHAT_PANEL_WIDTH = 520

type SideChatItem = {
  key: string
  role: string
  content: string
  status?: string
  contexts?: ComposerContext[]
  attachments?: BubbleAttachment[]
  at: string
}

type LiveSideChatTurn = {
  sideChatID: string
  baselineItemCount: number
  user: string
  contexts: ComposerContext[]
  attachments: BubbleAttachment[]
  at: string
}

export function SideChatPanel({
  events,
  visible,
  onSend,
  onUploadAttachment,
  onClose,
  fileRoot,
}: {
  events: SessionEvent[]
  visible: boolean
  onSend: (sideChatID: string, message: string, options?: SendMessageOptions) => Promise<void>
  onUploadAttachment?: (file: File) => Promise<Attachment>
  onClose: () => void
  fileRoot?: string
}) {
  const [sideChatID, setSideChatID] = useState(() => latestSideChatID(events) || newSideChatID())
  const [pending, setPending] = useState(false)
  const [pendingTurn, setPendingTurn] = useState<{ sideChatID: string; baselineItemCount: number } | null>(null)
  const [live, setLive] = useState<LiveSideChatTurn | null>(null)
  const [error, setError] = useState('')
  const scrollRef = useRef<HTMLDivElement>(null)
  const items = useMemo(() => sideChatItems(events, sideChatID), [events, sideChatID])
  const visibleItems = useMemo(() => {
    if (!live || live.sideChatID !== sideChatID) return items
    return [
      ...items,
      {
        key: `live:${live.at}`,
        role: 'user',
        content: live.user,
        contexts: live.contexts,
        attachments: live.attachments,
        at: live.at,
      },
    ]
  }, [items, live, sideChatID])
  const pendingActive = Boolean(pendingTurn && pendingTurn.sideChatID === sideChatID)
  const pendingOutputStarted = Boolean(
    pendingTurn &&
      pendingTurn.sideChatID === sideChatID &&
      items.slice(pendingTurn.baselineItemCount).some((item) => item.role !== 'user'),
  )
  const latestItemContent = visibleItems.at(-1)?.content

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
    if (items.length > live.baselineItemCount) setLive(null)
  }, [items.length, live, sideChatID])

  useEffect(() => {
    if (!visible) return
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight })
  }, [visible, visibleItems.length, latestItemContent, pendingActive, pendingOutputStarted])

  useEffect(() => {
    if (visibleItems.length > 0) setError('')
  }, [visibleItems.length])

  const close = () => {
    onClose()
    setError('')
  }

  const submit = (message: string, options?: SendMessageOptions) => {
    if (pending) return
    setError('')
    setPending(true)
    setPendingTurn({ sideChatID, baselineItemCount: items.length })
    setLive({
      sideChatID,
      baselineItemCount: items.length,
      user: message,
      contexts: options?.contexts ?? [],
      attachments: options?.attachments ?? [],
      at: new Date().toISOString(),
    })
    void onSend(sideChatID, message, options)
      .catch((err: Error) => {
        setError(err.message || 'Side chat failed.')
      })
      .finally(() => {
        setPending(false)
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
        <div ref={scrollRef} className="min-h-0 flex-1 overflow-y-auto bg-bg px-4 py-4">
          <div className="flex min-h-full flex-col justify-end gap-5">
            {visibleItems.map((item) => (
              <SideChatRow key={item.key} item={item} active={pendingActive} />
            ))}
            {pendingActive && !pendingOutputStarted ? (
              <p className="animate-pulse text-sm text-ink-3">Thinking…</p>
            ) : null}
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

function SideChatRow({ item, active }: { item: SideChatItem; active: boolean }) {
  if (item.role === 'user') {
    return <UserBubble text={item.content} contexts={item.contexts} attachments={item.attachments} />
  }
  if (item.role === 'assistant') {
    return <AssistantBubble text={item.content} />
  }
  if (item.role === 'error') {
    return <SessionErrorNotice message={item.content} />
  }
  if (item.role === 'thought') {
    return <ThinkingBlock text={item.content} pending={active && item.status === 'running'} />
  }
  if (item.role === 'tool') {
    return <ToolStatusLine label={item.content} status={item.status} active={active && item.status === 'running'} />
  }
  return <AssistantBubble text={item.content} />
}

function newSideChatID(): string {
  const raw = typeof crypto !== 'undefined' && crypto.randomUUID ? crypto.randomUUID() : String(Date.now())
  return `side_${raw.replaceAll('-', '')}`
}

function latestSideChatID(events: SessionEvent[]): string {
  const latest = events
    .filter((event) => event.type === 'side_chat_message' && event.side_chat?.id)
    .sort(compareEvents)
    .at(-1)
  return latest?.side_chat?.id ?? ''
}

function sideChatHasEvents(events: SessionEvent[], sideChatID: string): boolean {
  return events.some((event) => event.type === 'side_chat_message' && event.side_chat?.id === sideChatID)
}

function sideChatItems(events: SessionEvent[], sideChatID: string): SideChatItem[] {
  const sorted = [...events]
    .filter((event) => event.type === 'side_chat_message' && event.side_chat?.id === sideChatID)
    .sort(compareEvents)
  const out: SideChatItem[] = []
  sorted.forEach((event, index) => {
    const side = event.side_chat
    if (!side) return
    const content = side.content ?? event.content ?? ''
    const role = side.role || 'assistant'
    const previous = out.at(-1)
    if (previous && previous.role === role && (role === 'assistant' || role === 'thought')) {
      previous.content += content
      previous.at = event.at || previous.at
      previous.status = side.status || previous.status
      return
    }
    out.push({
      key: `${event.seq ?? index}:${role}:${event.at}`,
      role,
      content,
      status: side.status,
      contexts: sideChatContexts(side.contexts, event.seq ?? index),
      attachments: side.attachments,
      at: event.at,
    })
  })
  return out
}

function sideChatContexts(contexts: MessageContextInput[] = [], eventIndex: number): ComposerContext[] {
  return contexts.flatMap<ComposerContext>((context, index) => {
    if (context.type === 'selection') {
      return context.text ? [{ id: `${eventIndex}-selection-${index}`, type: 'selection', text: context.text }] : []
    }
    return context.browser_annotation
      ? [{
          id: `${eventIndex}-annotation-${index}`,
          type: 'browser_annotation',
          browser_annotation: context.browser_annotation,
        }]
      : []
  })
}

function compareEvents(a: SessionEvent, b: SessionEvent): number {
  if (a.seq && b.seq && a.session_id === b.session_id) return a.seq - b.seq
  const atA = Date.parse(a.at)
  const atB = Date.parse(b.at)
  return (Number.isNaN(atA) ? 0 : atA) - (Number.isNaN(atB) ? 0 : atB) || (a.seq ?? 0) - (b.seq ?? 0)
}
