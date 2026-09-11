import { createContext, useCallback, useContext, useLayoutEffect, useSyncExternalStore } from 'react'
import type { Attachment } from '@/lib/api/types'
import type { BrowserAnnotation } from '@/lib/messageContext'

export type PreviewTarget = { displayUrl: string; sourceUrl: string }
export type BrowserPresentation = {
  onClose: () => void
  onAddBrowserAnnotation?: (annotation: BrowserAnnotation, screenshot?: Attachment) => void
  onUploadAttachment?: (file: File) => Promise<Attachment>
}
export type BrowserSession = {
  id: string
  target: PreviewTarget
  presentation?: BrowserPresentation
}

const EMPTY_TARGET: PreviewTarget = { displayUrl: '', sourceUrl: '' }

export class BrowserSessions {
  private sessions: BrowserSession[] = []
  private listeners = new Set<() => void>()
  private viewers = new Map<string, () => void>()

  getSnapshot = (): BrowserSession[] => this.sessions

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener)
    return () => this.listeners.delete(listener)
  }

  bind(id: string, show: () => void): () => void {
    this.update(id, {})
    this.viewers.set(id, show)
    return () => {
      if (this.viewers.get(id) === show) this.viewers.delete(id)
    }
  }

  open(id: string, url: string): void {
    this.update(id, { target: { displayUrl: url, sourceUrl: url } })
    this.viewers.get(id)?.()
  }

  update(id: string, patch: Partial<Omit<BrowserSession, 'id'>>): void {
    const current = this.sessions.find((session) => session.id === id)
    const next = { id, target: EMPTY_TARGET, ...current, ...patch }
    this.sessions = current
      ? this.sessions.map((session) => session === current ? next : session)
      : [...this.sessions, next]
    this.listeners.forEach((listener) => listener())
  }

  present(id: string, presentation: BrowserPresentation): () => void {
    this.update(id, { presentation })
    return () => {
      if (this.sessions.find((session) => session.id === id)?.presentation === presentation) {
        this.update(id, { presentation: undefined })
      }
    }
  }

  clear(): void {
    this.sessions = []
    this.viewers.clear()
    this.listeners.forEach((listener) => listener())
  }
}

export const BrowserSessionsContext = createContext<BrowserSessions | null>(null)

export function useBrowserSessions(): BrowserSessions {
  const sessions = useContext(BrowserSessionsContext)
  if (!sessions) throw new Error('Browser sessions require BrowserWorkspace')
  return sessions
}

export function useSessionPreview(sessionId: string, show: () => void) {
  const sessions = useBrowserSessions()
  const getTarget = useCallback(() => sessions.getSnapshot().find((session) => session.id === sessionId)?.target ?? EMPTY_TARGET, [sessions, sessionId])
  const target = useSyncExternalStore(sessions.subscribe, getTarget)
  useLayoutEffect(() => sessions.bind(sessionId, show), [sessions, sessionId, show])
  const setTarget = useCallback((target: PreviewTarget) => {
    sessions.update(sessionId, { target })
  }, [sessions, sessionId])
  return { target, setTarget }
}
