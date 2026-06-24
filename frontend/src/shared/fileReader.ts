export interface FileReference {
  path: string
  line?: number
}

export interface FileReferenceMatch {
  raw: string
  index: number
  reference: FileReference
}

const FILE_LINE_SUFFIX = /^(.*?):(\d+)(?::\d+)?$/
const FILE_EXT = String.raw`\.[A-Za-z0-9][A-Za-z0-9]*`
const PATH_SEGMENT = String.raw`[^/\\\s<>(){}]+`
const FILE_NAME = `${PATH_SEGMENT}${FILE_EXT}`
const FILE_REFERENCE_LEFT_EDGE = /[\p{L}\p{N}_.~@%+-]/u
const FILE_REFERENCE_RIGHT_EDGE = /[\p{L}\p{N}_~@%+-]/u
const FILE_REFERENCE_PATTERN = new RegExp(
  String.raw`(?:` +
    [
      String.raw`file:\/\/[^\s<>(){}]+${FILE_EXT}`,
      String.raw`\/(?:${PATH_SEGMENT}\/)+${FILE_NAME}`,
      String.raw`[A-Za-z]:[\\/](?:${PATH_SEGMENT}[\\/])*${FILE_NAME}`,
      String.raw`[/\\]{2}${PATH_SEGMENT}[/\\]${PATH_SEGMENT}(?:[/\\]${PATH_SEGMENT})*[/\\]${FILE_NAME}`,
    ].join('|') +
    String.raw`)(?::\d+(?::\d+)?)?`,
  'g',
)

export function parseFileReference(value: string): FileReference | null {
  let path = value.trim().replace(/[),.;]+$/, '')
  if (!path) return null
  const lineMatch = FILE_LINE_SUFFIX.exec(path)
  const line = lineMatch ? Number(lineMatch[2]) : undefined
  if (lineMatch) path = lineMatch[1]
  if (!isAbsoluteFilePath(path)) return null
  return { path, line }
}

export function findFileReferences(value: string): FileReferenceMatch[] {
  if (!value.includes('/') && !value.includes('\\')) return []
  const out: FileReferenceMatch[] = []
  FILE_REFERENCE_PATTERN.lastIndex = 0
  for (const match of value.matchAll(FILE_REFERENCE_PATTERN)) {
    const raw = match[0]
    const index = match.index ?? 0
    if (!hasFileReferenceBoundary(value, index, raw)) continue
    const reference = parseFileReference(raw)
    if (!reference) continue
    out.push({ raw, index, reference })
  }
  return out
}

function hasFileReferenceBoundary(value: string, index: number, raw: string): boolean {
  if (index > 0) {
    const prev = value[index - 1]
    if (prev === '/' || prev === '\\') return false
    if ((raw.startsWith('//') || raw.startsWith('\\\\')) && prev === ':') return false
    if (FILE_REFERENCE_LEFT_EDGE.test(prev)) return false
  }
  const next = value[index + raw.length]
  if (next === undefined) return true
  if (next === '/' || next === '\\') return false
  if (FILE_REFERENCE_RIGHT_EDGE.test(next)) return false
  if (next !== '.') return true
  const afterDot = value[index + raw.length + 1]
  return afterDot === undefined || /\s|[),;!?]/.test(afterDot)
}

export function isAbsoluteFilePath(value: string): boolean {
  return (
    /^file:\/\//i.test(value) ||
    (value.startsWith('/') && !value.startsWith('//')) ||
    /^[A-Za-z]:[\\/]/.test(value) ||
    /^[/\\]{2}[^/\\]+[/\\][^/\\]+/.test(value)
  )
}

export function isHTMLFilePath(value: string): boolean {
  const lower = value.toLowerCase()
  return lower.endsWith('.html') || lower.endsWith('.htm')
}

export function shouldPreviewFileReference(file: FileReference): boolean {
  return file.line === undefined && isHTMLFilePath(file.path)
}
