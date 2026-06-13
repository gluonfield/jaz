export interface FileReference {
  path: string
  line?: number
}

const FILE_LINE_SUFFIX = /^(.*?):(\d+)(?::\d+)?$/

export function parseFileReference(value: string): FileReference | null {
  let path = value.trim().replace(/[),.;]+$/, '')
  if (!path) return null
  if (/^file:\/\//i.test(path)) {
    try {
      path = decodeURI(new URL(path).pathname)
    } catch {
      return null
    }
  }
  const lineMatch = FILE_LINE_SUFFIX.exec(path)
  const line = lineMatch ? Number(lineMatch[2]) : undefined
  if (lineMatch) path = lineMatch[1]
  if (!isAbsoluteFilePath(path)) return null
  return { path, line }
}

export function isAbsoluteFilePath(value: string): boolean {
  return value.startsWith('/') && !value.startsWith('//')
}
