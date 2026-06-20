import { useCallback, useLayoutEffect, useRef, useState } from 'react'
import type { ComposerQuote } from '@/lib/sendMessage'
import type { ComposerDraftStorage } from './useComposerDraft'

function quoteStore(kind: ComposerDraftStorage): Storage {
  return kind === 'local' ? localStorage : sessionStorage
}

function quoteKey(key: string | undefined): string {
  return key ? `${key}.quotes` : ''
}

function storedQuote(value: unknown): ComposerQuote | null {
  if (!value || typeof value !== 'object') return null
  const raw = value as Record<string, unknown>
  const id = typeof raw.id === 'string' ? raw.id : ''
  const text = typeof raw.text === 'string' ? raw.text : ''
  if (!id || !text) return null
  return { id, text }
}

function readQuotes(key: string | undefined, storage: ComposerDraftStorage): ComposerQuote[] {
  const storedKey = quoteKey(key)
  if (!storedKey) return []
  try {
    const parsed = JSON.parse(quoteStore(storage).getItem(storedKey) ?? '[]') as unknown
    if (!Array.isArray(parsed)) return []
    return parsed.flatMap((value) => {
      const quote = storedQuote(value)
      return quote ? [quote] : []
    })
  } catch {
    return []
  }
}

function writeQuotes(key: string | undefined, storage: ComposerDraftStorage, items: ComposerQuote[]): void {
  const storedKey = quoteKey(key)
  if (!storedKey) return
  try {
    if (items.length === 0) {
      quoteStore(storage).removeItem(storedKey)
      return
    }
    quoteStore(storage).setItem(storedKey, JSON.stringify(items))
  } catch {
    // Draft persistence must never block composing.
  }
}

export function useComposerQuotes({
  storageKey,
  storage,
  disabled,
}: {
  storageKey?: string
  storage: ComposerDraftStorage
  disabled?: boolean
}) {
  const [quotes, setQuotes] = useState<ComposerQuote[]>(() => readQuotes(storageKey, storage))
  const quotesRef = useRef(quotes)

  useLayoutEffect(() => {
    const next = readQuotes(storageKey, storage)
    quotesRef.current = next
    setQuotes(next)
  }, [storage, storageKey])

  const commitQuotes = useCallback((next: ComposerQuote[]) => {
    quotesRef.current = next
    setQuotes(next)
    writeQuotes(storageKey, storage, next)
  }, [storage, storageKey])

  const addQuote = useCallback((text: string) => {
    const trimmed = text.trim()
    if (disabled || !trimmed) return
    commitQuotes([...quotesRef.current, { id: crypto.randomUUID(), text: trimmed }])
  }, [commitQuotes, disabled])

  const removeQuote = useCallback((id: string) => {
    commitQuotes(quotesRef.current.filter((quote) => quote.id !== id))
  }, [commitQuotes])

  const clearQuotes = useCallback(() => {
    commitQuotes([])
  }, [commitQuotes])

  return { quotes, addQuote, removeQuote, clearQuotes }
}
