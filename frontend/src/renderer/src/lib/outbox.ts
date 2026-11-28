import { useSyncExternalStore } from 'react'
import type { Attachment, ChatMessage } from '@/lib/api/types'
import type { SendMessageOptions } from '@/lib/sendMessage'

export type OutboxChatMessage = Omit<ChatMessage, 'seq' | 'role' | 'delivery'> & { role: 'user' }

export interface PendingUserMessage {
  baselineMessageSeq: number
  message: OutboxChatMessage
}

// A message the person sent, kept until the server's copy of it is on screen.
// sending: the server has not accepted it. sent: accepted, its turn still
// streaming here. delivered: accepted, with nothing more to wait for. failed:
// the server never took it.
export interface OutboxMessage extends PendingUserMessage {
  id: string
  state: 'sending' | 'sent' | 'delivered' | 'failed'
  error?: string
}

// What a send needs to go out again.
export interface OutboxSend extends OutboxMessage {
  text: string
  options: SendMessageOptions
  uploaded: Attachment[]
}

export function pendingChatMessage(message: OutboxChatMessage, seq = 0): ChatMessage {
  return { ...message, seq, from_user: true }
}

// The transcript with every outbox message in it exactly once. An accepted
// message stands where the server's row for it is, found by its text among the
// person's messages saved after it was sent; until that row loads it stays at
// the end. A failed one never reached the server, so it has no row. `settled`
// lists the delivered messages whose rows are loaded: the outbox can let go of
// them without the transcript changing.
export function outboxTranscript(
  messages: ChatMessage[],
  outbox: OutboxMessage[],
): { messages: ChatMessage[]; settled: string[] } {
  const out = messages.slice()
  const claimed = new Set<number>()
  const settled: string[] = []
  let seq = (messages.at(-1)?.seq ?? 0) + 1_000_000
  for (const entry of outbox) {
    const text = entry.message.content.trim()
    const index = entry.state === 'failed' ? -1 : messages.findIndex((message, at) =>
      !claimed.has(at) &&
      message.seq > entry.baselineMessageSeq &&
      message.role === 'user' &&
      message.from_user === true &&
      message.content.trim() === text,
    )
    if (index !== -1) {
      claimed.add(index)
      // The send time stays the turn's start while its turn streams: the saved
      // row is stamped later than the turn's first events.
      if (entry.state === 'delivered') settled.push(entry.id)
      else out[index] = { ...messages[index], created_at: entry.message.created_at }
      continue
    }
    const message = pendingChatMessage(entry.message, seq++)
    out.push(entry.state === 'sending' || entry.state === 'failed'
      ? { ...message, delivery: { id: entry.id, state: entry.state, error: entry.error } }
      : message)
  }
  return { messages: out, settled }
}

// Outboxes outlive the thread view: a message sent just before the person
// opens another thread is still there, with its outcome, when they come back.
const boxes = new Map<string, OutboxSend[]>()
const listeners = new Set<() => void>()
const NONE: OutboxSend[] = []

function write(sessionId: string, next: OutboxSend[]) {
  if (next.length) boxes.set(sessionId, next)
  else boxes.delete(sessionId)
  for (const listener of listeners) listener()
}

export function outbox(sessionId: string): OutboxSend[] {
  return boxes.get(sessionId) ?? NONE
}

export function putOutboxMessage(sessionId: string, entry: OutboxSend) {
  const current = outbox(sessionId)
  write(sessionId, current.some((item) => item.id === entry.id)
    ? current.map((item) => (item.id === entry.id ? entry : item))
    : [...current, entry])
}

export function patchOutboxMessage(sessionId: string, id: string, patch: Partial<OutboxSend>) {
  const current = outbox(sessionId)
  if (current.some((item) => item.id === id)) {
    write(sessionId, current.map((item) => (item.id === id ? { ...item, ...patch } : item)))
  }
}

export function removeOutboxMessages(sessionId: string, ids: string[]) {
  const current = outbox(sessionId)
  const next = current.filter((item) => !ids.includes(item.id))
  if (next.length !== current.length) write(sessionId, next)
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

export function useOutbox(sessionId: string): OutboxSend[] {
  return useSyncExternalStore(subscribe, () => outbox(sessionId), () => outbox(sessionId))
}
