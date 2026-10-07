import type { ComposerDraftStorage } from '@/components/session/useComposerDraft'

const changes = new EventTarget()

export function subscribeComposerDraft(
  storage: ComposerDraftStorage,
  key: string | undefined,
  onChange: () => void,
): () => void {
  if (!key) {
    return () => {}
  }
  const event = JSON.stringify([storage, key])
  changes.addEventListener(event, onChange)
  return () => changes.removeEventListener(event, onChange)
}

export function notifyComposerDraft(storage: ComposerDraftStorage, key: string | undefined): void {
  if (key) {
    changes.dispatchEvent(new Event(JSON.stringify([storage, key])))
  }
}
