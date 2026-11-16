import type { PDFDocumentProxy } from 'pdfjs-dist'
import type { FileLocation } from '@shared/fileReader'

export function pdfTextMatch(text: string, quote: string): { start: number; end: number } | null {
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

export async function pdfLocation(document: PDFDocumentProxy, location?: FileLocation, signal?: AbortSignal): Promise<{ page?: number; quote?: string }> {
  const page = location?.page
  if (page && (page < 1 || page > document.numPages)) return {}
  if (!location?.text) return { page }
  for (let number = page ?? 1; number <= (page ?? document.numPages); number++) {
    if (signal?.aborted) return {}
    const content = await (await document.getPage(number)).getTextContent()
    const text = content.items.map((item) => 'str' in item ? item.str + (item.hasEOL ? '\n' : '') : '').join('')
    if (pdfTextMatch(text, location.text)) return { page: number, quote: location.text }
  }
  return { page }
}

export function pdfTextRange(root: HTMLElement, quote: string): Range | null {
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT | NodeFilter.SHOW_ELEMENT)
  const nodes: { node: Text; offset: number }[] = []
  let text = ''
  while (walker.nextNode()) {
    const node = walker.currentNode
    if (node.nodeType === Node.TEXT_NODE) {
      nodes.push({ node: node as Text, offset: text.length })
      text += node.textContent
    } else if (node.nodeName === 'BR') text += '\n'
  }
  const match = pdfTextMatch(text, quote)
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
