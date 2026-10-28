import { createContext, useContext, useLayoutEffect } from 'react'
import type { Attachment } from '@/lib/api/types'
import type { BrowserAnnotation } from '@/lib/messageContext'

export type PreviewTarget = { displayUrl: string; sourceUrl: string; title?: string; favicon?: string }
export type BrowserPresentation = {
  onClose?: () => void
  embedded?: boolean
  onAddBrowserAnnotation?: (annotation: BrowserAnnotation, screenshot?: Attachment) => void
  onUploadAttachment?: (file: File) => Promise<Attachment>
}
export type BrowserSession = {
  id: string
  ownerId?: string
  generation?: number
  reloads?: number
  embedded?: boolean
  target: PreviewTarget
  presentation?: BrowserPresentation
}

const EMPTY_TARGET: PreviewTarget = { displayUrl: '', sourceUrl: '' }

export class BrowserSessions {
  private sessions: BrowserSession[] = []
  private listeners = new Set<() => void>()
  private viewers = new Map<string, () => void | Promise<void>>()
  private pendingReveals = new Set<string>()

  getSnapshot = (): BrowserSession[] => this.sessions

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener)
    return () => this.listeners.delete(listener)
  }

  bind(id: string, show: () => void | Promise<void>): () => void {
    this.ensure(id)
    this.viewers.set(id, show)
    // Binding runs during React's commit; revealing can synchronously render the panel.
    queueMicrotask(() => {
      if (this.pendingReveals.has(id)) {
        this.show(id)
      }
    })
    return () => {
      if (this.viewers.get(id) === show) this.viewers.delete(id)
    }
  }

  ensure(id: string): void {
    if (!this.sessions.some((session) => session.id === id)) this.update(id, {})
  }

  open(id: string, url: string): void {
    this.update(id, { ownerId: id, target: { displayUrl: url, sourceUrl: url } })
    this.show(id)
  }

  show(id: string): void | Promise<void> {
    this.pendingReveals.delete(id)
    if (!this.sessions.find((session) => session.id === id)?.presentation) {
      const show = this.viewers.get(id)
      if (show) {
        return show()
      }
      this.pendingReveals.add(id)
    }
  }

  openTab(ownerId: string, url = ''): string {
    const existing = url && this.sessions.find((entry) => entry.ownerId === ownerId && entry.target.displayUrl === url)
    if (existing) {
      return existing.id
    }
    const primary = this.sessions.find((entry) => entry.id === ownerId)
    const id = primary?.ownerId ? crypto.randomUUID() : ownerId
    this.update(id, { ownerId, target: { displayUrl: url, sourceUrl: url } })
    return id
  }

  duplicate(ownerId: string, id: string): string {
    const copy = crypto.randomUUID()
    this.update(copy, { ownerId, target: this.sessions.find((session) => session.id === id)?.target ?? EMPTY_TARGET })
    return copy
  }

  reload(id: string): void {
    const entry = this.sessions.find((session) => session.id === id)
    if (entry) {
      this.update(id, { reloads: (entry.reloads ?? 0) + 1 })
    }
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
    this.update(id, { presentation, embedded: presentation.embedded })
    return () => {
      if (this.sessions.find((session) => session.id === id)?.presentation === presentation) {
        this.update(id, { presentation: undefined })
      }
    }
  }

  close(id: string): void {
    this.pendingReveals.delete(id)
    const entry = this.sessions.find((session) => session.id === id)
    if (!entry) {
      return
    }
    if (!entry.ownerId || entry.ownerId === id) {
      this.update(id, { ownerId: undefined, target: EMPTY_TARGET, presentation: undefined, generation: (entry.generation ?? 0) + 1 })
    } else {
      this.sessions = this.sessions.filter((session) => session.id !== id)
      this.listeners.forEach((listener) => listener())
    }
  }

  clear(): void {
    this.sessions = []
    this.viewers.clear()
    this.pendingReveals.clear()
    this.listeners.forEach((listener) => listener())
  }
}

export const BrowserSessionsContext = createContext<BrowserSessions | null>(null)

export function useBrowserSessions(): BrowserSessions {
  const sessions = useContext(BrowserSessionsContext)
  if (!sessions) throw new Error('Browser sessions require BrowserWorkspace')
  return sessions
}

export function useSessionPreview(sessionId: string, show: () => void | Promise<void>) {
  const sessions = useBrowserSessions()
  useLayoutEffect(() => sessions.bind(sessionId, show), [sessions, sessionId, show])
}
