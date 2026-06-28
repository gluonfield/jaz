import { useCallback, useLayoutEffect, useRef, useState } from 'react'
import type { Attachment } from '@/lib/api/types'
import { browserAnnotationFromUnknown, normalizeBrowserAnnotation } from '@/lib/messageContext'
import type { BrowserAnnotation, ComposerContext } from '@/lib/sendMessage'
import type { ComposerDraftStorage } from './useComposerDraft'

function contextStore(kind: ComposerDraftStorage): Storage {
  return kind === 'local' ? localStorage : sessionStorage
}

function contextKey(key: string | undefined): string {
  return key ? `${key}.contexts` : ''
}

function legacyQuoteKey(key: string | undefined): string {
  return key ? `${key}.quotes` : ''
}

function storedContext(value: unknown): ComposerContext | null {
  if (!value || typeof value !== 'object') return null
  const raw = value as Record<string, unknown>
  const id = typeof raw.id === 'string' ? raw.id : ''
  const type = typeof raw.type === 'string' ? raw.type : ''
  if (!id) return null
  if (type === 'selection') {
    const text = typeof raw.text === 'string' ? raw.text.trim() : ''
    if (!text) return null
    const comment = typeof raw.comment === 'string' ? raw.comment.trim() : ''
    return { id, type, text, comment: comment || undefined }
  }
  if (type === 'browser_annotation') {
    const annotation = browserAnnotationFromUnknown(raw.browser_annotation)
    return annotation ? { id, type, browser_annotation: annotation } : null
  }
  return null
}

function readContexts(key: string | undefined, storage: ComposerDraftStorage): ComposerContext[] {
  const storedKey = contextKey(key)
  if (!storedKey) return []
  try {
    const parsed = JSON.parse(contextStore(storage).getItem(storedKey) ?? '[]') as unknown
    if (Array.isArray(parsed)) {
      const contexts = parsed.flatMap((value) => {
        const context = storedContext(value)
        return context ? [context] : []
      })
      if (contexts.length) return contexts
    }
  } catch {
    // fall through to the legacy quote draft below
  }
  return readLegacyQuotes(key, storage)
}

function readLegacyQuotes(key: string | undefined, storage: ComposerDraftStorage): ComposerContext[] {
  const storedKey = legacyQuoteKey(key)
  if (!storedKey) return []
  try {
    const parsed = JSON.parse(contextStore(storage).getItem(storedKey) ?? '[]') as unknown
    if (!Array.isArray(parsed)) return []
    return parsed.flatMap((value) => {
      if (!value || typeof value !== 'object') return []
      const raw = value as Record<string, unknown>
      const text = typeof raw.text === 'string' ? raw.text.trim() : ''
      const id = typeof raw.id === 'string' && raw.id ? raw.id : crypto.randomUUID()
      return text ? [{ id, type: 'selection' as const, text }] : []
    })
  } catch {
    return []
  }
}

function writeContexts(key: string | undefined, storage: ComposerDraftStorage, items: ComposerContext[]): void {
  const storedKey = contextKey(key)
  if (!storedKey) return
  try {
    const persisted = items.flatMap((context) => {
      const next = persistedContext(context)
      return next ? [next] : []
    })
    if (persisted.length === 0) {
      contextStore(storage).removeItem(storedKey)
      const legacyKey = legacyQuoteKey(key)
      if (legacyKey) contextStore(storage).removeItem(legacyKey)
      return
    }
    contextStore(storage).setItem(storedKey, JSON.stringify(persisted))
  } catch {
    // Draft persistence must never block composing.
  }
}

function persistedContext(context: ComposerContext): ComposerContext | null {
  if (context.type === 'selection') {
    const text = context.text.trim()
    if (!text) return null
    return { id: context.id, type: 'selection', text, comment: context.comment?.trim() || undefined }
  }
  const annotation = normalizeBrowserAnnotation(context.browser_annotation)
  if (!annotation) return null
  const screenshotAttachmentID =
    context.screenshotAttachment?.id ?? annotation.screenshot_attachment_id
  return {
    id: context.id,
    type: 'browser_annotation',
    browser_annotation: {
      ...annotation,
      ...(screenshotAttachmentID ? { screenshot_attachment_id: screenshotAttachmentID } : {}),
    },
  }
}

export function useComposerContexts({
  storageKey,
  storage,
  disabled,
}: {
  storageKey?: string
  storage: ComposerDraftStorage
  disabled?: boolean
}) {
  const [contexts, setContexts] = useState<ComposerContext[]>(() => readContexts(storageKey, storage))
  const contextsRef = useRef(contexts)

  useLayoutEffect(() => {
    const next = readContexts(storageKey, storage)
    contextsRef.current = next
    setContexts(next)
  }, [storage, storageKey])

  const commitContexts = useCallback((next: ComposerContext[]) => {
    contextsRef.current = next
    setContexts(next)
    writeContexts(storageKey, storage, next)
  }, [storage, storageKey])

  const addSelection = useCallback((text: string, comment?: string) => {
    const trimmed = text.trim()
    if (disabled || !trimmed) return
    commitContexts([
      ...contextsRef.current,
      { id: crypto.randomUUID(), type: 'selection', text: trimmed, comment: comment?.trim() || undefined },
    ])
  }, [commitContexts, disabled])

  const addBrowserAnnotation = useCallback((annotation: BrowserAnnotation, screenshotAttachment?: Attachment) => {
    if (disabled) return
    const normalized = normalizeBrowserAnnotation(annotation)
    if (!normalized) return
    commitContexts([
      ...contextsRef.current,
      {
        id: crypto.randomUUID(),
        type: 'browser_annotation',
        browser_annotation: {
          ...normalized,
          ...(screenshotAttachment?.id ? { screenshot_attachment_id: screenshotAttachment.id } : {}),
        },
        screenshotAttachment,
      },
    ])
  }, [commitContexts, disabled])

  const removeContext = useCallback((id: string) => {
    commitContexts(contextsRef.current.filter((context) => context.id !== id))
  }, [commitContexts])

  const clearContexts = useCallback(() => {
    commitContexts([])
  }, [commitContexts])

  return { contexts, addSelection, addBrowserAnnotation, removeContext, clearContexts }
}
