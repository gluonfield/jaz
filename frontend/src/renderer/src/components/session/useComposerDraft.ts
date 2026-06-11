import { useCallback, useLayoutEffect, useState } from 'react'
import { pruneTokens, type InlineToken } from './composerTokens'

export type ComposerDraftStorage = 'session' | 'local'

export interface ComposerDraft {
  text: string
  tokens: Map<string, InlineToken>
}

type StoredComposerDraft = {
  text: string
  tokens: InlineToken[]
}

function emptyDraft(): ComposerDraft {
  return { text: '', tokens: new Map() }
}

function inlineTokenFrom(value: unknown): InlineToken | null {
  if (!value || typeof value !== 'object') return null
  const token = value as Record<string, unknown>
  const trigger = token.trigger
  const display = token.display
  const expansion = token.expansion
  if ((trigger !== '$' && trigger !== '@') || typeof display !== 'string' || typeof expansion !== 'string') return null
  return { trigger, display, expansion }
}

function tokenMap(tokens: InlineToken[]): Map<string, InlineToken> {
  return new Map(tokens.map((token) => [token.display, token]))
}

function draftStore(kind: ComposerDraftStorage): Storage {
  return kind === 'local' ? localStorage : sessionStorage
}

function readStoredDraft(key: string | undefined, kind: ComposerDraftStorage): ComposerDraft {
  if (!key) return emptyDraft()
  try {
    const raw = draftStore(kind).getItem(key)
    if (!raw) return emptyDraft()
    const parsed = JSON.parse(raw) as unknown
    if (!parsed || typeof parsed !== 'object') return emptyDraft()
    const draft = parsed as Record<string, unknown>
    const text = typeof draft.text === 'string' ? draft.text : ''
    const tokens = Array.isArray(draft.tokens)
      ? draft.tokens.flatMap((token) => {
          const parsedToken = inlineTokenFrom(token)
          return parsedToken ? [parsedToken] : []
        })
      : []
    return { text, tokens: tokenMap(tokens) }
  } catch {
    return emptyDraft()
  }
}

function storedDraftFrom(draft: ComposerDraft): StoredComposerDraft {
  return { text: draft.text, tokens: [...draft.tokens.values()] }
}

function writeStoredDraft(
  key: string | undefined,
  kind: ComposerDraftStorage,
  draft: ComposerDraft,
): void {
  if (!key) return
  try {
    if (!draft.text && draft.tokens.size === 0) {
      draftStore(kind).removeItem(key)
      return
    }
    draftStore(kind).setItem(key, JSON.stringify(storedDraftFrom(draft)))
  } catch {
    // Draft persistence must never block typing.
  }
}

function normalizedDraft(draft: ComposerDraft): ComposerDraft {
  const tokens = pruneTokens(draft.tokens, draft.text)
  return tokens === draft.tokens ? draft : { ...draft, tokens }
}

export function useComposerDraft({
  storageKey,
  storage = 'session',
  onTextChange,
}: {
  storageKey?: string
  storage?: ComposerDraftStorage
  onTextChange?: (text: string) => void
}) {
  const [draft, setDraftState] = useState(() => readStoredDraft(storageKey, storage))

  useLayoutEffect(() => {
    const next = readStoredDraft(storageKey, storage)
    setDraftState(next)
    onTextChange?.(next.text)
  }, [storage, storageKey, onTextChange])

  const setDraft = useCallback(
    (next: ComposerDraft) => {
      const draft = normalizedDraft(next)
      setDraftState(draft)
      onTextChange?.(draft.text)
      writeStoredDraft(storageKey, storage, draft)
    },
    [onTextChange, storage, storageKey],
  )

  const clearDraft = useCallback(() => setDraft(emptyDraft()), [setDraft])

  return {
    text: draft.text,
    tokens: draft.tokens,
    setDraft,
    clearDraft,
  }
}
