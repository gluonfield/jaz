import type { ComposerDraft, ComposerDraftStorage } from '@/components/session/useComposerDraft'
import type { ComposerAttachment } from '@/components/session/composerAttachmentTypes'
import type { ComposerContext } from '@/lib/sendMessage'

type ComposerDraftChange =
  | { field: 'text'; value: ComposerDraft }
  | { field: 'attachments'; value: ComposerAttachment[] }
  | { field: 'contexts'; value: ComposerContext[] }

const changes = new EventTarget()

export function subscribeComposerDraft(
  storage: ComposerDraftStorage,
  key: string | undefined,
  onChange: (change: ComposerDraftChange) => void,
): () => void {
  if (!key) {
    return () => {}
  }
  const event = JSON.stringify([storage, key])
  const listener = (event: Event) => onChange((event as CustomEvent<ComposerDraftChange>).detail)
  changes.addEventListener(event, listener)
  return () => changes.removeEventListener(event, listener)
}

export function notifyComposerDraft(
  storage: ComposerDraftStorage,
  key: string | undefined,
  change: ComposerDraftChange,
): void {
  if (key) {
    changes.dispatchEvent(new CustomEvent(JSON.stringify([storage, key]), { detail: change }))
  }
}
