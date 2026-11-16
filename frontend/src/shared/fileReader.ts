export interface FileLocation {
  page?: number
  sheet?: string
  range?: string
  line?: number
  text?: string
}

export interface FileReference {
  path: string
  location?: FileLocation
}

export interface FileReferenceMatch {
  raw: string
  index: number
  reference: FileReference
}

export function isHTMLPath(path: string): boolean {
  return /\.html?(?:[?#].*)?$/i.test(path)
}

export function isSpreadsheetPath(path: string): boolean {
  return /\.(csv|tsv|xlsx|xls|xlsm|xlsb|ods)(?:[?#].*)?$/i.test(path)
}

export function isMarkdownPath(path: string): boolean {
  return /\.(md|markdown)(?:[?#].*)?$/i.test(path)
}

export function resolveFileLink(value: string, documentPath?: string): FileReference | null {
  const hash = value.indexOf('#')
  const fragment = hash < 0 ? '' : value.slice(hash + 1)
  const target = (hash < 0 ? value : value.slice(0, hash)).trim()
  const line = FILE_LINE_SUFFIX.exec(target)
  let path = line ? line[1] : target
  path = decoded(path)
  if (!path || !isAbsoluteFilePath(path) && (/^[a-z][a-z\d+.-]*:|^#/i.test(path) || !line && !/\.[a-z\d]+$/i.test(path))) return null
  const directory = documentPath?.slice(0, Math.max(documentPath.lastIndexOf('/'), documentPath.lastIndexOf('\\')) + 1) ?? ''
  path = normalizeFilePath(isAbsoluteFilePath(path) ? path : directory + path)
  const location: FileLocation = line ? { line: Number(line[2]) } : {}
  const params = new URLSearchParams(fragment)
  const page = Number(params.get('page'))
  if (Number.isSafeInteger(page) && page > 0) location.page = page
  const text = params.get('text')
  if (text) location.text = text
  if (isSpreadsheetPath(path) && fragment) {
    const selection = decoded(fragment)
    const split = selection.lastIndexOf('!')
    const range = split < 0 ? selection : selection.slice(split + 1)
    if (/^\$?[a-z]+\$?[1-9]\d*(?::\$?[a-z]+\$?[1-9]\d*)?$/i.test(range)) {
      location.range = range.toUpperCase()
      if (split >= 0) location.sheet = selection.slice(0, split).replace(/^'(.*)'$/, '$1').replace(/''/g, "'")
    }
  }
  if (Object.keys(location).length) return { path, location }
  return { path: isHTMLPath(path) && fragment ? path + '#' + fragment : path }
}

function decoded(value: string): string {
  try {
    return decodeURIComponent(value)
  } catch {
    return value
  }
}

function normalizeFilePath(path: string): string {
  if (/^file:\/\//i.test(path)) {
    try { return new URL(path).href } catch { return path }
  }
  const prefix = /^(?:[a-z]:[\\/]|[\\/]{1,2})/i.exec(path)?.[0] ?? ''
  const separator = path.includes('\\') ? '\\' : '/'
  const parts: string[] = []
  for (const part of path.slice(prefix.length).split(/[\\/]/)) {
    if (!part || part === '.') continue
    if (part === '..' && parts.length && parts.at(-1) !== '..') parts.pop()
    else if (part !== '..' || !prefix) parts.push(part)
  }
  return prefix + parts.join(separator)
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
    String.raw`)(?::\d+(?::\d+)?)?(?:#[^\s<>(){}]+)?`,
  'g',
)

export function parseFileReference(value: string): FileReference | null {
  const target = value.trim().replace(/[),.;]+$/, '')
  const reference = resolveFileLink(target)
  return reference && isAbsoluteFilePath(reference.path) ? reference : null
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
