import type { ChatMessage, SessionMessages } from '@/lib/api/types'
import { coalesceSessionEvents } from '@/lib/sessionEvents'

export function mergeLatestHistory(current: SessionMessages | undefined, latest: SessionMessages): SessionMessages {
  if (!current || current.history_revision !== latest.history_revision || !historyOverlaps(current, latest)) {
    return latest
  }
  return {
    ...latest,
    messages: mergeMessages(current.messages, latest.messages),
    events: coalesceSessionEvents([...current.events, ...latest.events]),
    acp_meta: { ...current.acp_meta, ...latest.acp_meta },
    has_earlier: current.has_earlier,
    before_message_seq: olderCursor(current.before_message_seq, latest.before_message_seq),
    before_event_seq: olderCursor(current.before_event_seq, latest.before_event_seq),
  }
}

export function mergeEarlierHistory(current: SessionMessages, earlier: SessionMessages): SessionMessages {
  return {
    ...current,
    messages: mergeMessages(earlier.messages, current.messages),
    events: coalesceSessionEvents([...earlier.events, ...current.events]),
    acp_meta: { ...earlier.acp_meta, ...current.acp_meta },
    has_earlier: earlier.has_earlier,
    before_message_seq: earlier.before_message_seq,
    before_event_seq: earlier.before_event_seq,
  }
}

function mergeMessages(...groups: ChatMessage[][]): ChatMessage[] {
  const messages = new Map<number, ChatMessage>()
  for (const group of groups) {
    for (const message of group) messages.set(message.seq, message)
  }
  return [...messages.values()].sort((a, b) => a.seq - b.seq)
}

function historyOverlaps(current: SessionMessages, latest: SessionMessages): boolean {
  if (!current.messages.length || !latest.messages.length) return true
  return current.messages.at(-1)!.seq >= latest.messages[0].seq
}

function olderCursor(current?: number, latest?: number): number | undefined {
  if (!current) return latest
  if (!latest) return current
  return Math.min(current, latest)
}
