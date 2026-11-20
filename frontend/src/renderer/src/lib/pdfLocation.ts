import type { PDFDocumentProxy } from 'pdfjs-dist'
import type { FileLocation } from '@shared/fileReader'

import { fileTextMatch } from '@/lib/fileTextLocation'

export async function pdfLocation(document: PDFDocumentProxy, location?: FileLocation, signal?: AbortSignal): Promise<{ page?: number; quote?: string }> {
  const page = location?.page
  if (page && (page < 1 || page > document.numPages)) return {}
  if (!location?.text) return { page }
  for (let number = page ?? 1; number <= (page ?? document.numPages); number++) {
    if (signal?.aborted) return {}
    const content = await (await document.getPage(number)).getTextContent()
    const text = content.items.map((item) => 'str' in item ? item.str + (item.hasEOL ? '\n' : '') : '').join('')
    if (fileTextMatch(text, location.text)) return { page: number, quote: location.text }
  }
  return { page }
}
