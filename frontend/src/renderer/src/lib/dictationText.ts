export function dictationText(text: string, start: number, end: number, transcript: string): string {
  const spoken = transcript.trim()
  if (!spoken) {
    return ''
  }
  const before = text.slice(0, start)
  const after = text.slice(end)
  const prefix = before && !/[\s([{]$/.test(before) && !/^[,.;:!?)\]}]/.test(spoken) ? ' ' : ''
  const suffix = after && !/^[\s,.;:!?)}\]]/.test(after) ? ' ' : ''
  return prefix + spoken + suffix
}
