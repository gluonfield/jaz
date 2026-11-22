import { expect, test } from 'bun:test'
import { CFB, read, utils, write } from 'xlsx'
import { cellTextBounds, initialSpreadsheetSelection, moveSpreadsheetSelection, readSpreadsheet, selectionRange, selectionTSV, spreadsheetCell, spreadsheetLayout, spreadsheetStyle, spreadsheetText, visibleIndices } from './spreadsheet'
import { spreadsheetMetadata } from './spreadsheetMetadata'

const load = (data, digit = 7) => readSpreadsheet(data, () => digit)

const PNG = Uint8Array.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 1, 2, 3])

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
      xml = xml.replace('<sheetData>', '<sheetViews><sheetView showGridLines="0" workbookViewId="0"><pane xSplit="1" ySplit="1" state="frozen" topLeftCell="B2"/></sheetView></sheetViews><sheetData>')
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
  const relationship = (type, target) => `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/${type}" Target="${target}"/></Relationships>`
  const parts = {
    'xl/worksheets/_rels/sheet1.xml.rels': relationship('drawing', '../drawings/drawing1.xml'),
    'xl/drawings/_rels/drawing1.xml.rels': relationship('image', '../media/image1.png'),
    'xl/drawings/drawing1.xml': '<xdr:wsDr xmlns:xdr="http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><xdr:oneCellAnchor><xdr:from><xdr:col>1</xdr:col><xdr:colOff>95250</xdr:colOff><xdr:row>2</xdr:row><xdr:rowOff>0</xdr:rowOff></xdr:from><xdr:ext cx="190500" cy="95250"/><xdr:pic><xdr:blipFill><a:blip r:embed="rId1"/></xdr:blipFill><xdr:spPr><a:xfrm><a:ext cx="1" cy="1"/></a:xfrm></xdr:spPr></xdr:pic></xdr:oneCellAnchor></xdr:wsDr>',
    'xl/media/image1.png': PNG,
  }
  for (const [name, content] of Object.entries(parts)) CFB.utils.cfb_add(zip, `Root Entry/${name}`, typeof content === 'string' ? new TextEncoder().encode(content) : content)
  return CFB.write(zip, { type: 'buffer', fileType: 'zip' })
}

test('CSV preserves text, leading zeroes and quoted newlines; copy produces escaped TSV', () => {
  const sheet = load(new TextEncoder().encode('Name,Code,Note\r\n"Motor, large",00123,"line one\nline two"\r\n1-2,00456,plain').buffer)[0]
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
  const loaded = load(save())
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
  const sheet = load(data)[0]
  expect(sheet.frozen).toEqual({ r: 1, c: 1 })
  expect(sheet.gridlines).toBe(false)
  expect(sheet.images).toEqual([{ type: 'image/png', data: PNG, left: sheet.columns[1] + 10, top: sheet.rows[2], width: 20, height: 10 }])
  expect(spreadsheetLayout(sheet, 264, 64).frozen).toEqual({ r: 1, c: 1 })
  expect(spreadsheetLayout(sheet, 263, 63).frozen).toEqual({ r: 0, c: 0 })
  expect(sheet.columns[1]).toBe(132)
  expect(load(data, 9)[0].columns[1]).toBe(169)
  expect(sheet.rows[3] - sheet.rows[2]).toBeCloseTo(80)
  expect(visibleIndices(sheet.rows, 0, Infinity)).toEqual([0, 1, 2, 3])
  expect(spreadsheetStyle(sheet, { r: 0, c: 0 })).toMatchObject({ bold: true, color: '#17365D', horizontal: 'center', vertical: 'center', wrap: false, border: { left: '1px solid #000', top: '2px solid #FF0000', bottom: '1px dashed #000' } })
  expect(spreadsheetStyle(sheet, { r: 2, c: 0 })).toMatchObject({ wrap: true, vertical: 'top' })
})

test('nonwrapped text overflows styled empty neighbours, stops at content, and wrapping stays within its cell', () => {
  const sheet = load(styledWorkbook())[0]
  expect(cellTextBounds(sheet, utils.decode_range('A2')).width).toBe(sheet.columns[2])
  expect(cellTextBounds(sheet, utils.decode_range('A3')).width).toBe(sheet.columns[1])
  expect(cellTextBounds(sheet, utils.decode_range('A1')).width).toBe(sheet.columns[1])
})

test('selection expands merges, arrows skip merged and hidden cells, shift extends the anchor', () => {
  const sheet = load(styledWorkbook())[0]
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
  const sheet = load(new TextEncoder().encode(csv).buffer)[0]
  expect(sheet.rows).toHaveLength(50001)
  expect(spreadsheetText(sheet, { r: 49999, c: 1 })).toBe('value')
  expect(visibleIndices(sheet.rows, 800000, 800500)).toHaveLength(25)
  const filled = spreadsheetLayout(load(workbook([['Small']]).save())[0], 1200, 900)
  expect(filled.columns.at(-1)).toBeGreaterThanOrEqual(1200)
  expect(filled.rows.at(-1)).toBeGreaterThanOrEqual(900)
  expect(filled.used).toEqual(utils.decode_range('A1'))
})


function legacyWorkbook(rows, { globals = Buffer.alloc(0), sheet = Buffer.alloc(0), patch = () => {} }) {
  const container = CFB.read(new Uint8Array(workbook(rows, 'xls').save()), { type: 'buffer' })
  const source = Buffer.from(CFB.find(container, '/Workbook').content)
  const ends = []
  let bound = 0
  for (let offset = 0; offset + 4 <= source.length; offset += source.readUInt16LE(offset + 2) + 4) {
    const id = source.readUInt16LE(offset)
    if (id === 0x0085) bound = offset + 4
    if (id === 0x000a) ends.push(offset)
    patch(source, id, offset + 4)
  }
  source.writeUInt32LE(source.readUInt32LE(bound) + globals.length, bound)
  CFB.utils.cfb_add(container, `${container.FullPaths[0]}Workbook`, Buffer.concat([source.subarray(0, ends[0]), globals, source.subarray(ends[0], ends[1]), sheet, source.subarray(ends[1])]))
  return load(CFB.write(container, { type: 'buffer', fileType: 'cfb' }))[0]
}

function words(...values) {
  return Buffer.from(values.flatMap((value) => [value & 0xff, value >> 8]))
}

function record(id, body) {
  const header = Buffer.alloc(4)
  header.writeUInt16LE(id, 0)
  header.writeUInt16LE(body.length, 2)
  return Buffer.concat([header, body])
}

function art(type, version, ...children) {
  const body = Buffer.concat(children.map((child) => Buffer.from(child)))
  const header = Buffer.alloc(8)
  header.writeUInt16LE(version, 0)
  header.writeUInt16LE(type, 2)
  header.writeUInt32LE(body.length, 4)
  return Buffer.concat([header, body])
}

test.each([true, false])('legacy XLS distinguishes frozen panes from ordinary splits (frozen=%s)', (frozen) => {
  const sheet = legacyWorkbook([['A', 'B'], ['C', 'D'], ['E', 'F']], { sheet: Buffer.concat([record(0x023e, words(frozen ? 0x0008 : 0, 0, 0, 0, 0, 0, 0, 0, 0)), record(0x0041, words(1, 2, 0, 0, 0))]) })
  expect(sheet.frozen).toEqual(frozen ? { r: 2, c: 1 } : { r: 0, c: 0 })
  expect(sheet.gridlines).toBe(false)
  expect(spreadsheetText(sheet, { r: 2, c: 1 })).toBe('F')
})

test('legacy XLS recovers fonts, alignment, borders and pictures anchored to cells', () => {
  const picture = art(0xf007, 0x0002, Buffer.alloc(36), art(0xf01e, 0x6e00, Buffer.alloc(17), PNG))
  const anchor = art(0xf010, 0x0000, words(0, 1, 512, 0, 128, 2, 0, 1, 0))
  const shape = art(0xf004, 0x000f, art(0xf00b, 0x0013, Buffer.from([0x04, 0x41, 1, 0, 0, 0])), anchor)
  const sheet = legacyWorkbook([['Centred', 'B'], ['C', 'D']], {
    globals: record(0x00eb, art(0xf000, 0x000f, art(0xf001, 0x001f, picture))),
    sheet: record(0x00ec, art(0xf002, 0x000f, art(0xf003, 0x000f, shape))),
    patch: (source, id, offset) => {
      if (id !== 0x00e0) return
      source.writeUInt8(0x1a, offset + 6)
      source.writeUInt32LE(0x0020 | 10 << 23, offset + 10)
    },
  })
  expect(spreadsheetStyle(sheet, { r: 0, c: 0 })).toMatchObject({ fontFamily: 'Arial', horizontal: 'center', vertical: 'center', wrap: true, border: { left: undefined, right: '2px solid #FF0000' } })
  expect(sheet.images).toEqual([{ type: 'image/png', data: PNG, left: sheet.columns[1] + (sheet.columns[2] - sheet.columns[1]) / 2, top: sheet.rows[1] / 2, width: (sheet.columns[2] - sheet.columns[1]) / 2, height: sheet.rows[1] / 2 }])
})
