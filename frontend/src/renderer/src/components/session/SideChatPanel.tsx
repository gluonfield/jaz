import { ArrowUp, LoaderCircle, MessageSquarePlus, X } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { IconButton } from '@/components/ui/IconButton'
import type { Session, SessionEvent } from '@/lib/api/types'
import { MessageMarkdown } from './MessageMarkdown'
import { SidePanelShell } from './SidePanelShell'

export const SIDE_CHAT_PANEL_WIDTH = 520

const SIDE_CHAT_ID_KEY_PREFIX = 'jaz.sideChat.'

type SideChatMessage = {
  key: string
  role: string
  content: string
  status?: string
  at: string
}

export function SideChatPanel({
  session,
  events,
  visible,
  onSend,
  onClose,
}: {
  session: Session
  events: SessionEvent[]
  visible: boolean
  onSend: (sideChatID: string, message: string) => Promise<void>
  onClose: () => void
}) {
  const storageKey = `${SIDE_CHAT_ID_KEY_PREFIX}${session.id}`
  const [sideChatID, setSideChatID] = useState(() => initialSideChatID(storageKey, events))
  const [draft, setDraft] = useState('')
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const scrollRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)
  const storageKeyRef = useRef(storageKey)
  const messages = useMemo(() => sideChatMessages(events, sideChatID), [events, sideChatID])
  const latestMessageContent = messages.at(-1)?.content

  useEffect(() => {
    if (storageKeyRef.current === storageKey) return
    storageKeyRef.current = storageKey
    setSideChatID(initialSideChatID(storageKey, events))
    setDraft('')
    setError('')
  }, [storageKey, events])

  useEffect(() => {
    localStorage.setItem(storageKey, sideChatID)
  }, [storageKey, sideChatID])

  useEffect(() => {
    if (!visible) return
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight })
  }, [visible, messages.length, latestMessageContent])

  const startNew = () => {
    setSideChatID(newSideChatID())
    setDraft('')
    setError('')
    requestAnimationFrame(() => inputRef.current?.focus())
  }

  const submit = () => {
    const message = draft.trim()
    if (!message || pending) return
    setDraft('')
    setError('')
    setPending(true)
    onSend(sideChatID, message)
      .catch((err: Error) => {
        setError(err.message || 'Side chat failed.')
        setDraft(message)
      })
      .finally(() => setPending(false))
  }

  return (
    <SidePanelShell width={SIDE_CHAT_PANEL_WIDTH}>
      <div className="flex h-11 shrink-0 items-center justify-between border-b border-border px-3">
        <div className="min-w-0 text-sm font-medium text-ink">Side chat</div>
        <div className="flex items-center gap-1">
          {pending ? <LoaderCircle size={15} className="animate-spin text-ink-3" aria-hidden /> : null}
          <IconButton size="sm" aria-label="New side chat" title="New side chat" onClick={startNew}>
            <MessageSquarePlus size={15} />
          </IconButton>
          <IconButton size="sm" aria-label="Hide side panel" title="Hide side panel" onClick={onClose}>
            <X size={15} />
          </IconButton>
        </div>
      </div>
      <div ref={scrollRef} className="min-h-0 flex-1 overflow-y-auto px-3 py-3">
        <div className="flex min-h-full flex-col justify-end gap-3">
          {messages.map((message) => (
            <SideChatBubble key={message.key} message={message} />
          ))}
        </div>
      </div>
      {error ? <div className="px-3 pb-2 text-[12px] text-danger">{error}</div> : null}
      <div className="shrink-0 border-t border-border p-2">
        <div className="flex items-end gap-2 rounded-[12px] bg-bg p-2">
          <textarea
            ref={inputRef}
            value={draft}
            rows={2}
            placeholder="Ask in side chat"
            disabled={pending}
            onChange={(event) => setDraft(event.currentTarget.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter' && !event.shiftKey) {
                event.preventDefault()
                submit()
              }
            }}
            className="max-h-32 min-h-10 flex-1 resize-none bg-transparent px-1 py-1 text-sm leading-5 text-ink outline-none placeholder:text-ink-3 disabled:opacity-60"
          />
          <IconButton
            variant="primary"
            size="md"
            aria-label="Send side chat"
            title="Send side chat"
            disabled={pending || draft.trim() === ''}
            onClick={submit}
          >
            <ArrowUp size={16} />
          </IconButton>
        </div>
      </div>
    </SidePanelShell>
  )
}

function SideChatBubble({ message }: { message: SideChatMessage }) {
  if (message.role === 'user') {
    return (
      <div className="flex justify-end">
        <div className="min-w-0 max-w-[88%] rounded-card bg-bg px-3 py-2 text-sm whitespace-pre-wrap [overflow-wrap:break-word] text-ink select-text">
          {message.content}
        </div>
      </div>
    )
  }
  if (message.role === 'error') {
    return (
      <div className="rounded-card border border-danger/20 bg-danger-soft px-3 py-2 text-sm text-danger">
        {message.content}
      </div>
    )
  }
  if (message.role === 'thought') {
    return <div className="text-[12px] leading-5 whitespace-pre-wrap text-ink-3">{message.content}</div>
  }
  if (message.role === 'tool') {
    return (
      <div className="inline-flex max-w-full self-start rounded-full bg-bg px-2.5 py-1 text-[12px] text-ink-3">
        <span className="truncate">{message.content}</span>
        {message.status ? <span className="ml-1.5 shrink-0">- {message.status}</span> : null}
      </div>
    )
  }
  return (
    <div className="min-w-0 max-w-full text-sm text-ink">
      <MessageMarkdown text={message.content} />
    </div>
  )
}

function initialSideChatID(storageKey: string, events: SessionEvent[]): string {
  return localStorage.getItem(storageKey) || latestSideChatID(events) || newSideChatID()
}

function latestSideChatID(events: SessionEvent[]): string {
  for (let i = events.length - 1; i >= 0; i--) {
    const id = events[i].side_chat?.id
    if (id) return id
  }
  return ''
}

function newSideChatID(): string {
  const raw = typeof crypto !== 'undefined' && crypto.randomUUID ? crypto.randomUUID() : String(Date.now())
  return `side_${raw.replaceAll('-', '')}`
}

function sideChatMessages(events: SessionEvent[], sideChatID: string): SideChatMessage[] {
  const sorted = [...events]
    .filter((event) => event.type === 'side_chat_message' && event.side_chat?.id === sideChatID)
    .sort(compareEvents)
  const out: SideChatMessage[] = []
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
      at: event.at,
    })
  })
  return out
}

function compareEvents(a: SessionEvent, b: SessionEvent): number {
  if (a.seq && b.seq && a.session_id === b.session_id) return a.seq - b.seq
  const atA = Date.parse(a.at)
  const atB = Date.parse(b.at)
  return (Number.isNaN(atA) ? 0 : atA) - (Number.isNaN(atB) ? 0 : atB) || (a.seq ?? 0) - (b.seq ?? 0)
}
