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

export function mergeEarlierHistory(current: SessionMessages, ...pages: SessionMessages[]): SessionMessages {
  if (!pages.length) return current
  const boundary = pages.at(-1)!
  const chronological = pages.reverse()
  return {
    ...current,
    messages: mergeMessages(...chronological.map((page) => page.messages), current.messages),
    events: coalesceSessionEvents([
      ...chronological.flatMap((page) => page.events),
      ...current.events,
    ]),
    acp_meta: Object.assign({}, ...chronological.map((page) => page.acp_meta), current.acp_meta),
    has_earlier: boundary.has_earlier,
    before_message_seq: boundary.before_message_seq,
    before_event_seq: boundary.before_event_seq,
  }
}

export async function loadCompleteHistoryBatch(
  current: SessionMessages,
  loadPage: (cursor: SessionMessages) => Promise<SessionMessages>,
): Promise<SessionMessages> {
  const pages: SessionMessages[] = []
  let cursor = current
  while (cursor.has_earlier) {
    if (!cursor.before_message_seq && !cursor.before_event_seq) {
      throw new Error('Earlier history has no continuation cursor')
    }
    const page = await loadPage(cursor)
    if (page.history_revision !== cursor.history_revision) {
      throw new Error('Earlier history revision changed')
    }
    if (
      page.has_earlier &&
      page.before_message_seq === cursor.before_message_seq &&
      page.before_event_seq === cursor.before_event_seq
    ) {
      throw new Error('Earlier history cursor did not advance')
    }
    pages.push(page)
    cursor = page
    if (page.messages.some((message) => message.role === 'user')) break
  }
  return mergeEarlierHistory(current, ...pages)
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
