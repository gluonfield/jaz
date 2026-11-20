export function fileTextMatch(text: string, quote: string): { start: number; end: number } | null {
  const words = quote.match(/\S+/g)
  if (!words) return null
  const pattern = words.map((word) => word.replace(/[$.*+?^{}()|[\]\\]/g, '\\$&')).join('\\s+')
  const match = new RegExp(pattern, 'iu').exec(text)
  return match && { start: match.index, end: match.index + match[0].length }
}

export function fileTextRange(root: HTMLElement, quote: string): Range | null {
  const document = root.ownerDocument
  const view = document.defaultView
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT | NodeFilter.SHOW_ELEMENT, (node) =>
    node.nodeType === Node.ELEMENT_NODE && view?.getComputedStyle(node as Element).display === 'none'
      ? NodeFilter.FILTER_REJECT : NodeFilter.FILTER_ACCEPT)
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
