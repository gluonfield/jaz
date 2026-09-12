import { sessionFileRawUrl } from '@/lib/api/sessions'
import { isAbsoluteFilePath } from '@shared/fileReader'

export function markdownImageSource(src: string, sessionId?: string, documentPath?: string): string {
  if (src.startsWith('//')) {
    return 'https:' + src
  }
  if (/^https?:\/\//i.test(src) || /^data:image\/(?:png|jpe?g|gif|webp|avif|bmp|svg\+xml)[;,]/i.test(src)) {
    return src
  }
  if (!sessionId || !src || src.startsWith('#')) {
    return ''
  }
  if (/^file:/i.test(src)) {
    return sessionFileRawUrl(sessionId, src)
  }
  let path: string
  try {
    path = decodeURIComponent(src)
  } catch {
    return ''
  }
  if (/^[a-z][a-z\d+.-]*:/i.test(path) && !/^[a-z]:[\\/]/i.test(path)) {
    return ''
  }
  if (documentPath && !isAbsoluteFilePath(path)) {
    const directory = documentPath.slice(0, Math.max(documentPath.lastIndexOf('/'), documentPath.lastIndexOf('\\')) + 1)
    path = directory + path
  }
  return sessionFileRawUrl(sessionId, path)
}
