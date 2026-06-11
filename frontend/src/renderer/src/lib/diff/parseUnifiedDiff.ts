export interface DiffLine {
  kind: 'context' | 'add' | 'del'
  oldNo?: number
  newNo?: number
  text: string
}

export interface DiffHunk {
  // The trailing function/section hint from the @@ header, when git found one.
  context: string
  lines: DiffLine[]
}

const HUNK_HEADER = /^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@ ?(.*)$/

// Parses a single-file unified diff into hunks with old/new line numbers.
// Hunk line counts from the @@ header decide when a hunk ends — prefix
// sniffing alone would misread content lines that start with "---"/"+++".
export function parseUnifiedDiff(patch: string): DiffHunk[] {
  const hunks: DiffHunk[] = []
  let hunk: DiffHunk | null = null
  let oldNo = 0
  let newNo = 0
  let oldLeft = 0
  let newLeft = 0
  for (const line of patch.split('\n')) {
    if (oldLeft <= 0 && newLeft <= 0) {
      const header = HUNK_HEADER.exec(line)
      if (header) {
        oldNo = Number(header[1])
        newNo = Number(header[3])
        oldLeft = Number(header[2] ?? '1')
        newLeft = Number(header[4] ?? '1')
        hunk = { context: header[5] ?? '', lines: [] }
        hunks.push(hunk)
      }
      // Anything else out here is file-header noise (diff --git, index, ±±±).
      continue
    }
    if (!hunk) continue
    const marker = line[0]
    const text = line.slice(1)
    if (marker === '+') {
      hunk.lines.push({ kind: 'add', newNo: newNo++, text })
      newLeft--
    } else if (marker === '-') {
      hunk.lines.push({ kind: 'del', oldNo: oldNo++, text })
      oldLeft--
    } else if (marker === '\\') {
      // "\ No newline at end of file" — metadata, not content. Dropped
      // consistently: when it trails the patch it lands in the header scan
      // above and is skipped there too.
    } else {
      hunk.lines.push({ kind: 'context', oldNo: oldNo++, newNo: newNo++, text })
      oldLeft--
      newLeft--
    }
  }
  return hunks
}
