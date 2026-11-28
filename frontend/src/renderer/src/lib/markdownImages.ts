import { sessionFileRawUrl } from '@/lib/api/sessions'
import { isAbsoluteFilePath } from '@shared/fileReader'

function withImageParam(url: string, imageId: string): string {
  const sep = url.includes('?') ? '&' : '?'
  return `${url}${sep}image=${encodeURIComponent(imageId)}`
}

export function markdownImageSource(
  src: string,
  imageId: string,
  sessionId?: string,
  documentPath?: string,
  cloudRunId?: string,
): string {
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
    return withImageParam(sessionFileRawUrl(sessionId, src, cloudRunId), imageId)
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
  return withImageParam(sessionFileRawUrl(sessionId, path, cloudRunId), imageId)
}
