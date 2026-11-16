import { expect, test } from 'bun:test'
import { getDocument } from 'pdfjs-dist/legacy/build/pdf.mjs'
import { pdfLocation } from './pdfLocation'

function pdf(pages) {
  const objects = [
    '<< /Type /Catalog /Pages 2 0 R >>',
    `<< /Type /Pages /Count ${pages.length} /Kids [${pages.map((_, i) => `${4 + i * 2} 0 R`).join(' ')}] >>`,
    '<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>',
  ]
  for (let i = 0; i < pages.length; i++) {
    const stream = `BT /F1 12 Tf 50 700 Td ${pages[i]} ET`
    objects.push(`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 3 0 R >> >> /Contents ${5 + i * 2} 0 R >>`)
    objects.push(`<< /Length ${stream.length} >>\nstream\n${stream}\nendstream`)
  }
  let source = '%PDF-1.4\n'
  const offsets = [0]
  for (let i = 0; i < objects.length; i++) {
    offsets.push(source.length)
    source += `${i + 1} 0 obj\n${objects[i]}\nendobj\n`
  }
  const start = source.length
  source += `xref\n0 ${offsets.length}\n0000000000 65535 f \n`
  source += offsets.slice(1).map((offset) => `${String(offset).padStart(10, '0')} 00000 n \n`).join('')
  source += `trailer\n<< /Size ${offsets.length} /Root 1 0 R >>\nstartxref\n${start}\n%%EOF`
  return new TextEncoder().encode(source)
}

test('PDF locations use real extracted page text without wrapping a scoped search', async () => {
  const task = getDocument({ data: pdf([
    '(Opening page) Tj',
    '(Liquidated) Tj 70 0 Td (   damages) Tj 0 -20 Td (must be paid) Tj',
    '(Liquidated damages must be paid) Tj',
    '',
  ]) })
  try {
    const document = await task.promise
    expect(document.numPages).toBe(4)
    expect(await pdfLocation(document, { page: 3, text: 'liquidated damages must be paid' })).toEqual({ page: 3, quote: 'liquidated damages must be paid' })
    expect(await pdfLocation(document, { text: 'liquidated\n  damages must be paid' })).toEqual({ page: 2, quote: 'liquidated\n  damages must be paid' })
    expect(await pdfLocation(document, { page: 1, text: 'liquidated damages' })).toEqual({ page: 1 })
    expect(await pdfLocation(document, { page: 4, text: 'liquidated damages' })).toEqual({ page: 4 })
    expect(await pdfLocation(document, { text: 'absent quotation' })).toEqual({ page: undefined })
    expect(await pdfLocation(document, { page: 20, text: 'liquidated damages' })).toEqual({})
  } finally {
    await task.destroy()
  }
})
