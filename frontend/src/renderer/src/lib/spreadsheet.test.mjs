import { expect, test } from 'bun:test'
import { CFB, read, utils, write } from 'xlsx'
import { cellTextBounds, initialSpreadsheetSelection, moveSpreadsheetSelection, readSpreadsheet, selectionRange, selectionTSV, spreadsheetCell, spreadsheetLayout, spreadsheetStyle, spreadsheetText, visibleIndices } from './spreadsheet'
import { spreadsheetMetadata } from './spreadsheetMetadata'

function workbook(rows, bookType = 'xlsx') {
  const book = utils.book_new()
  utils.book_append_sheet(book, utils.aoa_to_sheet(rows), 'Data')
  return { book, sheet: book.Sheets.Data, save: () => write(book, { type: 'array', bookType }) }
}

function styledWorkbook() {
  const { sheet, save } = workbook([['Heading', 'Other', 'Value'], ['Long text', '', 12.5], ['Wrapped words over several lines', '', 45658], ['Merged title'], ['A', 'B', 'C']])
  sheet.C2.z = '0.00'
  sheet.C2.f = 'SUM(10,2.5)'
  sheet.C3.z = 'dd/mm/yyyy'
  sheet['!merges'] = [{ s: { r: 3, c: 0 }, e: { r: 3, c: 2 } }]
  sheet['!cols'] = [{ wch: 18 }, { wch: 14 }, { wch: 12 }]
  sheet['!rows'] = [{ hpt: 24 }, { hpt: 20 }, { hpt: 60 }, {}, { hidden: true }]
  const zip = CFB.read(new Uint8Array(save()), { type: 'buffer' })
  for (const name of ['xl/worksheets/sheet1.xml', 'xl/styles.xml']) {
    const file = CFB.find(zip, `Root Entry/${name}`)
    let xml = new TextDecoder().decode(file.content)
    if (name.includes('sheet1')) {
      xml = xml.replace('<sheetData>', '<sheetViews><sheetView workbookViewId="0"><pane xSplit="1" ySplit="1" state="frozen" topLeftCell="B2"/></sheetView></sheetViews><sheetData>')
      for (const [address, style] of [['A1', 3], ['B2', 3], ['A3', 4]]) {
        xml = xml.replace(new RegExp(`<c [^>]*r="${address}"[^>]*>`), (tag) => tag.replace(/ s="\d+"/g, '').replace('<c', `<c s="${style}"`))
      }
    } else {
      xml = xml.replace('<fonts count="1">', '<fonts count="2"><font><b/><sz val="12"/><color rgb="FF17365D"/></font>')
      xml = xml.replace('</borders>', '<border><left style="thin"><color indexed="64"/></left><right/><top style="medium"><color rgb="FFFF0000"/></top><bottom style="dashed"/><diagonal/></border></borders>')
      xml = xml.replace('<cellXfs count="3">', '<cellXfs count="5">').replace('</cellXfs>', '<xf fontId="0" fillId="0" borderId="1" numFmtId="0"><alignment horizontal="center" vertical="center"/></xf><xf fontId="1" fillId="0" numFmtId="0"><alignment wrapText="1" vertical="top"/></xf></cellXfs>')
    }
    CFB.utils.cfb_add(zip, `Root Entry/${name}`, new TextEncoder().encode(xml))
  }
  return CFB.write(zip, { type: 'buffer', fileType: 'zip' })
}

test('CSV preserves text, leading zeroes and quoted newlines; copy produces escaped TSV', () => {
  const sheet = readSpreadsheet(new TextEncoder().encode('Name,Code,Note\r\n"Motor, large",00123,"line one\nline two"\r\n1-2,00456,plain').buffer)[0]
  expect(spreadsheetText(sheet, { r: 2, c: 0 })).toBe('1-2')
  expect(selectionTSV(sheet, initialSpreadsheetSelection(sheet, 'A2:C3'))).toBe('Motor, large\t00123\t"line one\nline two"\n1-2\t00456\tplain')
})

test.each(['xlsx', 'xls'])('%s preserves all sheets, cached formula values, dates, dimensions and merges', (bookType) => {
  const { book, sheet, save } = workbook([['Part', 'Cost', 'Date'], ['Motor', 12.5, 45658], ['Merged'] ], bookType)
  sheet.B2.z = '$0.00'
  sheet.B2.f = 'SUM(10,2.5)'
  sheet.C2.z = 'dd/mm/yyyy'
  sheet['!cols'] = [{ wch: 20 }, { hidden: true }, { wch: 12 }]
  sheet['!rows'] = [{ hpt: 30 }]
  sheet['!merges'] = [utils.decode_range('A3:C3')]
  utils.book_append_sheet(book, utils.aoa_to_sheet([[500]]), 'Other')
  const loaded = readSpreadsheet(save())
  expect(loaded.map((sheet) => sheet.name)).toEqual(['Data', 'Other'])
  expect(spreadsheetText(loaded[0], { r: 1, c: 1 })).toBe('$12.50')
  if (bookType === 'xlsx') expect(spreadsheetCell(loaded[0], { r: 1, c: 1 }).f).toBe('SUM(10,2.5)')
  expect(spreadsheetText(loaded[0], { r: 1, c: 2 })).toBe('01/01/2025')
  if (bookType === 'xlsx') expect(loaded[0].rows[1]).toBe(40)
  expect(loaded[0].columns[2]).toBe(loaded[0].columns[1])
  expect(selectionTSV(loaded[0], initialSpreadsheetSelection(loaded[0], 'B3'))).toBe('Merged\t\t')
})

test('XLSX recovers frozen panes, font, alignment, borders and explicit wrap without altering SheetJS metadata', () => {
  const data = styledWorkbook()
  const raw = read(data, { type: 'array', dense: true, cellStyles: true, bookFiles: true })
  const before = structuredClone(raw.Styles)
  const cells = structuredClone(raw.Sheets.Data['!data'])
  spreadsheetMetadata(raw)
  expect(raw.Styles).toEqual(before)
  expect(raw.Sheets.Data['!data']).toEqual(cells)
  const sheet = readSpreadsheet(data)[0]
  expect(sheet.frozen).toEqual({ r: 1, c: 1 })
  expect(sheet.rows[3] - sheet.rows[2]).toBeCloseTo(80)
  expect(visibleIndices(sheet.rows, 0, Infinity)).toEqual([0, 1, 2, 3])
  expect(spreadsheetStyle(sheet, { r: 0, c: 0 })).toMatchObject({ bold: true, color: '#17365D', horizontal: 'center', vertical: 'center', wrap: false, border: { left: '1px solid #000', top: '2px solid #FF0000', bottom: '1px dashed #000' } })
  expect(spreadsheetStyle(sheet, { r: 2, c: 0 })).toMatchObject({ wrap: true, vertical: 'top' })
})

test('nonwrapped text overflows styled empty neighbours, stops at content, and wrapping stays within its cell', () => {
  const sheet = readSpreadsheet(styledWorkbook())[0]
  expect(cellTextBounds(sheet, utils.decode_range('A2')).width).toBe(sheet.columns[2])
  expect(cellTextBounds(sheet, utils.decode_range('A3')).width).toBe(sheet.columns[1])
  expect(cellTextBounds(sheet, utils.decode_range('A1')).width).toBe(sheet.columns[1])
})

test('selection expands merges, arrows skip merged and hidden cells, shift extends the anchor', () => {
  const sheet = readSpreadsheet(styledWorkbook())[0]
  let selection = initialSpreadsheetSelection(sheet, 'C4')
  expect(selectionRange(sheet, selection)).toEqual(utils.decode_range('A4:C4'))
  selection = moveSpreadsheetSelection(sheet, selection, 'ArrowUp', false, false)
  expect(selection).toEqual({ anchor: { r: 2, c: 2 }, focus: { r: 2, c: 2 } })
  selection = moveSpreadsheetSelection(sheet, selection, 'ArrowLeft', true, false)
  expect(selectionRange(sheet, selection)).toEqual(utils.decode_range('B3:C3'))
  const grid = spreadsheetLayout(sheet, 700, 500)
  expect(moveSpreadsheetSelection(grid, initialSpreadsheetSelection(grid, 'A4'), 'Tab', false, false).focus).toEqual({ r: 3, c: 3 })
  const last = initialSpreadsheetSelection(sheet, 'A4')
  expect(moveSpreadsheetSelection(sheet, last, 'ArrowDown', false, false)).toEqual(last)
})

test('large CSV has no row cap and virtualization visits only visible cells; blank grid fills the viewport', () => {
  const csv = Array.from({ length: 50000 }, (_, index) => `${index},value`).join('\n')
  const sheet = readSpreadsheet(new TextEncoder().encode(csv).buffer)[0]
  expect(sheet.rows).toHaveLength(50001)
  expect(spreadsheetText(sheet, { r: 49999, c: 1 })).toBe('value')
  expect(visibleIndices(sheet.rows, 800000, 800500)).toHaveLength(25)
  const filled = spreadsheetLayout(readSpreadsheet(workbook([['Small']]).save())[0], 1200, 900)
  expect(filled.columns.at(-1)).toBeGreaterThanOrEqual(1200)
  expect(filled.rows.at(-1)).toBeGreaterThanOrEqual(900)
  expect(filled.used).toEqual(utils.decode_range('A1'))
})


test.each([true, false])('legacy XLS distinguishes frozen panes from ordinary splits (frozen=%s)', (frozen) => {
  const { save } = workbook([['A', 'B'], ['C', 'D'], ['E', 'F']], 'xls')
  const container = CFB.read(new Uint8Array(save()), { type: 'buffer' })
  const entry = CFB.find(container, '/Workbook')
  const source = Buffer.from(entry.content)
  let sheetStart = 0
  for (let offset = 0; offset + 4 < source.length; offset += source.readUInt16LE(offset + 2) + 4) {
    if (source.readUInt16LE(offset) === 0x0085) {
      sheetStart = source.readUInt32LE(offset + 4)
      break
    }
  }
  let end = sheetStart
  while (source.readUInt16LE(end) !== 0x000a) end += source.readUInt16LE(end + 2) + 4
  const records = Buffer.alloc(36)
  records.writeUInt16LE(0x023e, 0)
  records.writeUInt16LE(18, 2)
  records.writeUInt16LE(frozen ? 0x0008 : 0, 4)
  records.writeUInt16LE(0x0041, 22)
  records.writeUInt16LE(10, 24)
  records.writeUInt16LE(1, 26)
  records.writeUInt16LE(2, 28)
  CFB.utils.cfb_add(container, `${container.FullPaths[0]}Workbook`, Buffer.concat([source.subarray(0, end), records, source.subarray(end)]))
  const sheet = readSpreadsheet(CFB.write(container, { type: 'buffer', fileType: 'cfb' }))[0]
  expect(sheet.frozen).toEqual(frozen ? { r: 2, c: 1 } : { r: 0, c: 0 })
  expect(spreadsheetText(sheet, { r: 2, c: 1 })).toBe('F')
})
