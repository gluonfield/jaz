import { canvasLink } from '@/lib/canvasNavigation'
import { parseFileReference, resolveFileLink, type FileReference } from '@shared/fileReader'

export type MarkdownLinkTarget =
  | { kind: 'canvas'; sessionId: string; page?: string; item?: string }
  | { kind: 'anchor'; id: string }
  | { kind: 'file'; file: FileReference }
  | { kind: 'url'; url: string }

// markdownLinkTarget is where a link written in Markdown leads: a canvas item,
// a heading of its own document, a file the reader opens, or a web page. files
// is the thread and document the Markdown belongs to, when files can be opened;
// a link whose address is no file may still name one in its label.
export function markdownLinkTarget(href: string | undefined, label: string, files: { sessionId: string; documentPath?: string } | null): MarkdownLinkTarget | null {
  const canvas = canvasLink(href, files?.sessionId)
  if (canvas) return { kind: 'canvas', ...canvas }
  if (href?.startsWith('#')) return { kind: 'anchor', id: decoded(href.slice(1), decodeURIComponent) }
  const file = files && ((href ? resolveFileLink(decoded(href, decodeURI), files.documentPath) : null) ?? parseFileReference(label.trim()))
  if (file) return { kind: 'file', file }
  return href && /^https?:\/\//i.test(href) ? { kind: 'url', url: href } : null
}

// The Markdown pipeline percent-encodes addresses; a malformed one stays as written.
export function decoded(value: string, decode: (value: string) => string): string {
  try {
    return decode(value)
  } catch {
    return value
  }
}

// A plain primary click follows a link here; a modified one is the browser's.
export function isPlainClick(event: Pick<MouseEvent, 'button' | 'metaKey' | 'ctrlKey' | 'shiftKey' | 'altKey' | 'defaultPrevented'>): boolean {
  return event.button === 0 && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey && !event.defaultPrevented
}
