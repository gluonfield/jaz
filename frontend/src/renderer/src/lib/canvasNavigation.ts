import { getCanvas } from '@/lib/api/canvas'
import { readSessionFile } from '@/lib/api/sessions'

declare module '@tanstack/history' {
  interface HistoryState {
    canvasNavigation?: string
  }
}

export function canvasLink(href: string | undefined, sessionId?: string): { sessionId: string; page?: string; item?: string } | null {
  if (!href || (!href.startsWith('/sessions/') && !(sessionId && href.startsWith('?')))) return null
  try {
    const url = new URL(href, `https://jaz.invalid/sessions/${encodeURIComponent(sessionId ?? '')}`)
    const match = /^\/sessions\/([^/]+)$/.exec(url.pathname)
    if (!match || (!url.searchParams.has('page') && !url.searchParams.has('item'))) return null
    return { sessionId: decodeURIComponent(match[1]), page: url.searchParams.get('page') ?? undefined, item: url.searchParams.get('item') ?? undefined }
  } catch {
    return null
  }
}

export async function isCanvasFile(sessionId: string, path: string): Promise<boolean> {
  const [file, canvas] = await Promise.all([readSessionFile(sessionId, path), getCanvas(sessionId)])
  return file.path === `${canvas.dir}/canvas.json`
}
