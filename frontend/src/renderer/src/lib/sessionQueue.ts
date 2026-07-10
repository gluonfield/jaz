import type { QueuedAction, QueuedMessage, QueuedMessageInput } from '@/lib/api/types'
import { normalizeBrowserAnnotation } from '@/lib/messageContext'

export function normalizeQueuedMessagesForDisplay(prompts: QueuedMessage[]): QueuedMessage[] {
  return prompts.flatMap((prompt, index) => {
    const normalized = normalizeQueuedMessageInput(prompt)
    if (!normalized) return []
    return [{ ...normalized, id: normalized.id?.trim() || `legacy-${index}` }]
  })
}

export function normalizeQueuedMessageInput(prompt: QueuedMessageInput): QueuedMessageInput | null {
  if (prompt.action) {
    return {
      ...(prompt.id?.trim() ? { id: prompt.id.trim() } : {}),
      ...queuedActionMessage(prompt.action, prompt.text),
    }
  }
  const text = prompt.text.trim()
  const contexts = normalizeContexts(prompt.contexts)
  const legacyQuotes = (prompt.quotes ?? []).map((quote) => quote.trim()).filter(Boolean)
  const attachmentIds = (prompt.attachment_ids ?? []).map((id) => id.trim()).filter(Boolean)
  if (!text && contexts.length === 0 && legacyQuotes.length === 0 && attachmentIds.length === 0) {
    return null
  }
  return {
    ...(prompt.id?.trim() ? { id: prompt.id.trim() } : {}),
    text,
    ...(contexts.length || legacyQuotes.length
      ? { contexts: [...legacyQuotes.map((text) => ({ type: 'selection' as const, text })), ...contexts] }
      : {}),
    ...(attachmentIds.length ? { attachment_ids: attachmentIds } : {}),
    ...(prompt.plan_requested ? { plan_requested: true } : {}),
    ...(prompt.goal_requested ? { goal_requested: true } : {}),
  }
}

export function queuedActionMessage(action: QueuedAction, label = ''): QueuedMessageInput {
  return { action, text: label.trim() || queuedActionLabel(action) }
}

function normalizeContexts(contexts: QueuedMessageInput['contexts'] = []): NonNullable<QueuedMessageInput['contexts']> {
  return contexts.flatMap<NonNullable<QueuedMessageInput['contexts']>[number]>((context) => {
    if (context.type === 'selection') {
      const text = context.text?.trim()
      if (!text) return []
      return [{ type: 'selection' as const, text, comment: context.comment?.trim() || undefined }]
    }
    if (context.type !== 'browser_annotation' || !context.browser_annotation) return []
    const annotation = normalizeBrowserAnnotation(context.browser_annotation)
    return annotation ? [{ type: 'browser_annotation' as const, browser_annotation: annotation }] : []
  })
}

function queuedActionLabel(action: QueuedAction): string {
  switch (action) {
    case 'archive':
      return 'Archive thread'
    case 'unarchive':
      return 'Unarchive thread'
    case 'compact':
      return 'Compact'
    case 'repo/commit':
      return 'Commit changes'
    case 'repo/push':
      return 'Push branch'
    case 'repo/merge':
      return 'Hand off to main'
    case 'repo/merge-from-main':
      return 'Update from main'
    case 'repo/restore-worktree':
      return 'Restore worktree'
  }
}
