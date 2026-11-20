export function fileTextMatch(text: string, quote: string): { start: number; end: number } | null {
  const query = quote.replace(/\s+/g, ' ').trim().toLowerCase()
  if (!query) return null
  let normalized = ''
  const offsets: number[] = []
  for (let i = 0; i < text.length; i++) {
    const char = /\s/.test(text[i]) ? ' ' : text[i].toLowerCase()
    if (char === ' ' && (!normalized || normalized.endsWith(' '))) continue
    normalized += char
    for (let j = 0; j < char.length; j++) offsets.push(i)
  }
  const start = normalized.indexOf(query)
  return start < 0 ? null : { start: offsets[start], end: offsets[start + query.length - 1] + 1 }
}

export function fileTextRange(root: HTMLElement, quote: string): Range | null {
  const document = root.ownerDocument
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT | NodeFilter.SHOW_ELEMENT)
  const nodes: { node: Text; offset: number }[] = []
  let text = ''
  while (walker.nextNode()) {
    const node = walker.currentNode
    if (node.nodeType === Node.TEXT_NODE) {
      nodes.push({ node: node as Text, offset: text.length })
      text += node.textContent
    } else if (node.nodeName === 'BR' || node.nodeName === 'P') text += '\n'
  }
  const match = fileTextMatch(text, quote)
  if (!match) return null
  const range = document.createRange()
  for (const { node, offset } of nodes) {
    if (offset <= match.start && match.start < offset + node.length) range.setStart(node, match.start - offset)
    if (offset < match.end && match.end <= offset + node.length) {
      range.setEnd(node, match.end - offset)
      return range
    }
  }
  return null
}
