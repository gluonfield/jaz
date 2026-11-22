import { memo, useEffect, useLayoutEffect, useMemo, useRef, useCallback, useId, useState, type CSSProperties, type KeyboardEvent } from 'react'
import { utils, type CellAddress, type Range } from 'xlsx'
import { cellTextBounds, initialSpreadsheetSelection, mergedRange, moveSpreadsheetSelection, selectionRange, selectionTSV, spreadsheetCell, spreadsheetIndex, spreadsheetLayout, spreadsheetStyle, spreadsheetText, visibleIndices, type SpreadsheetSelection, type SpreadsheetSheet } from '@/lib/spreadsheet'
import type { SpreadsheetStyle } from '@/lib/spreadsheetMetadata'
import '@/components/session/spreadsheet.css'

const ROW_HEADER = 46
const COLUMN_HEADER = 26
const DEFAULT_FONT_SIZE = 12

function fontFamily(style: SpreadsheetStyle, fallback: string) {
  return style.fontFamily ? `${JSON.stringify(style.fontFamily)}, ${fallback}` : fallback
}

export function spreadsheetDigitWidth(font: SpreadsheetStyle): number {
  const context = document.createElement('canvas').getContext('2d')!
  context.font = `${font.fontSize ?? DEFAULT_FONT_SIZE}px ${fontFamily(font, getComputedStyle(document.body).fontFamily)}`
  return Math.round(Math.max(...[...'0123456789'].map((digit) => context.measureText(digit).width)))
}

function rectangle(sheet: SpreadsheetSheet, range: Range): CSSProperties {
  return {
    left: sheet.columns[range.s.c],
    top: sheet.rows[range.s.r],
    width: sheet.columns[range.e.c + 1] - sheet.columns[range.s.c],
    height: sheet.rows[range.e.r + 1] - sheet.rows[range.s.r],
  }
}

const Cell = memo(function Cell({ sheet, r, c, active, id }: { id: string; sheet: SpreadsheetSheet; r: number; c: number; active: boolean }) {
  const range = mergedRange(sheet, { r, c })
  const style = spreadsheetStyle(sheet, range.s)
  const cell = spreadsheetCell(sheet, range.s)
  const address = utils.encode_cell(range.s)
  const text = spreadsheetText(sheet, range.s)
  const textBounds = text ? cellTextBounds(sheet, range) : undefined
  return <div
    id={id}
    role="gridcell"
    aria-label={`${address}: ${text}`}
    aria-selected={active}
    aria-readonly="true"
    aria-rowindex={range.s.r + 1}
    aria-colindex={range.s.c + 1}
    aria-rowspan={range.e.r - range.s.r + 1}
    aria-colspan={range.e.c - range.s.c + 1}
    className="spreadsheet-cell"
    style={{ ...rectangle(sheet, range), backgroundColor: style.fill, color: style.color, fontWeight: style.bold ? 600 : 400, fontStyle: style.italic ? 'italic' : undefined, fontSize: style.fontSize ?? DEFAULT_FONT_SIZE, fontFamily: fontFamily(style, 'var(--font-sans)'), textAlign: style.horizontal ?? (cell?.t === 'n' ? 'right' : 'left') }}
  >
    {style.border && <div className="spreadsheet-cell-border" style={{ left: range.s.c ? -1 : 0, top: range.s.r ? -1 : 0, borderTop: style.border.top, borderRight: style.border.right, borderBottom: style.border.bottom, borderLeft: style.border.left }} />}
    {textBounds && <div className="spreadsheet-cell-text" style={{ ...textBounds, justifyContent: style.vertical === 'top' ? 'flex-start' : style.vertical === 'center' ? 'center' : 'flex-end' }}>
      <span style={{ width: sheet.columns[range.e.c + 1] - sheet.columns[range.s.c], marginLeft: -textBounds.left, whiteSpace: style.wrap ? 'pre-wrap' : 'pre', overflowWrap: style.wrap ? 'anywhere' : undefined }}>{style.wrap ? text : text.replace(/[\r\n]+/g, ' ')}</span>
    </div>}
  </div>
})

function Pane({ id, sheet, rows, columns, left, top, width, height, scrollLeft, scrollTop, selection }: {
  id: string
  sheet: SpreadsheetSheet
  rows: number[]
  columns: number[]
  left: number
  top: number
  width: number
  height: number
  scrollLeft: number
  scrollTop: number
  selection: SpreadsheetSelection
}) {
  const cells = new Map<string, Range>()
  for (const r of rows) {
    for (const c of columns) {
      const range = mergedRange(sheet, { r, c })
      cells.set(utils.encode_cell(range.s), range)
    }
  }
  for (const r of rows) {
    const source = sheet.source['!data']?.[r]
    for (let c = Math.min((columns[0] ?? 0) - 1, (source?.length ?? 0) - 1); c >= 0; c -= 1) {
      if (source?.[c]?.v === undefined || source[c].v === '') continue
      const range = mergedRange(sheet, { r, c })
      cells.set(utils.encode_cell(range.s), range)
      break
    }
  }
  const selected = selectionRange(sheet, selection)
  const active = mergedRange(sheet, selection.anchor)
  const cellRows = new Map<number, Range[]>()
  for (const cell of cells.values()) {
    const row = cellRows.get(cell.s.r) ?? []
    row.push(cell)
    cellRows.set(cell.s.r, row)
  }
  return <div role="rowgroup" className="spreadsheet-pane" style={{ left, top, width, height }}>
    <div style={{ position: 'absolute', left: -scrollLeft, top: -scrollTop }}>
      {[...cellRows].map(([r, cells]) => <div key={r} role="row" aria-rowindex={r + 1}>
        {cells.map((range) => <Cell key={utils.encode_cell(range.s)} id={`${id}-${utils.encode_cell(range.s)}`} sheet={sheet} r={range.s.r} c={range.s.c} active={range.s.r >= selected.s.r && range.s.r <= selected.e.r && range.s.c >= selected.s.c && range.s.c <= selected.e.c} />)}
      </div>)}
      <div className="spreadsheet-selection" style={rectangle(sheet, selected)} />
      <div className="spreadsheet-active" style={rectangle(sheet, active)} />
    </div>
  </div>
}

export default function SpreadsheetGrid({ sheet: source, initialSelection }: { sheet: SpreadsheetSheet; initialSelection?: string }) {
  const id = useId()
  const scroll = useRef<HTMLDivElement>(null)
  const drag = useRef<{ x: number; y: number } | null>(null)
  const [dragging, setDragging] = useState(false)
  const [selection, setSelection] = useState(() => initialSpreadsheetSelection(source, initialSelection))
  const [viewport, setViewport] = useState({ width: 0, height: 0, left: 0, top: 0 })
  const sheet = useMemo(() => spreadsheetLayout(source, viewport.width - ROW_HEADER, viewport.height - COLUMN_HEADER), [source, viewport.width, viewport.height])
  const frozenWidth = sheet.columns[sheet.frozen.c]
  const frozenHeight = sheet.rows[sheet.frozen.r]
  const range = selectionRange(sheet, selection)
  const activeRange = mergedRange(sheet, selection.anchor)
  const activePane = (activeRange.s.r < sheet.frozen.r ? 2 : 0) + (activeRange.s.c < sheet.frozen.c ? 1 : 0)
  const active = spreadsheetCell(sheet, activeRange.s)
  const address = utils.encode_range(range)
  const fullValue = active?.f ? `=${active.f}` : active?.v === undefined ? '' : String(active.v)
  const rows = visibleIndices(sheet.rows, viewport.top + frozenHeight, viewport.top + viewport.height - COLUMN_HEADER + 100)
  const columns = visibleIndices(sheet.columns, viewport.left + frozenWidth, viewport.left + viewport.width - ROW_HEADER + 100)
  const frozenRows = visibleIndices(sheet.rows, 0, Math.min(frozenHeight, viewport.height))
  const frozenColumns = visibleIndices(sheet.columns, 0, Math.min(frozenWidth, viewport.width))
  const panes = [
    { rows, columns, left: ROW_HEADER + frozenWidth, top: COLUMN_HEADER + frozenHeight, width: viewport.width - ROW_HEADER - frozenWidth, height: viewport.height - COLUMN_HEADER - frozenHeight, scrollLeft: viewport.left + frozenWidth, scrollTop: viewport.top + frozenHeight },
    { rows, columns: frozenColumns, left: ROW_HEADER, top: COLUMN_HEADER + frozenHeight, width: frozenWidth, height: viewport.height - COLUMN_HEADER - frozenHeight, scrollLeft: 0, scrollTop: viewport.top + frozenHeight },
    { rows: frozenRows, columns, left: ROW_HEADER + frozenWidth, top: COLUMN_HEADER, width: viewport.width - ROW_HEADER - frozenWidth, height: frozenHeight, scrollLeft: viewport.left + frozenWidth, scrollTop: 0 },
    { rows: frozenRows, columns: frozenColumns, left: ROW_HEADER, top: COLUMN_HEADER, width: frozenWidth, height: frozenHeight, scrollLeft: 0, scrollTop: 0 },
  ]

  const activeVisible = panes[activePane].rows.some((r) => r >= activeRange.s.r && r <= activeRange.e.r) && panes[activePane].columns.some((c) => c >= activeRange.s.c && c <= activeRange.e.c)

  useLayoutEffect(() => {
    const element = scroll.current!
    const update = () => setViewport({ width: element.clientWidth, height: element.clientHeight, left: element.scrollLeft, top: element.scrollTop })
    update()
    const observer = new ResizeObserver(update)
    observer.observe(element)
    return () => observer.disconnect()
  }, [])

  useLayoutEffect(() => {
    if (dragging) return
    const element = scroll.current!
    const target = mergedRange(sheet, selection.focus)
    const reveal = (axis: number[], start: number, end: number, frozen: number, current: number, extent: number) => {
      if (start < frozen) return current
      if (axis[start] < current + axis[frozen]) return axis[start] - axis[frozen]
      if (axis[end + 1] > current + extent) return Math.min(axis[start] - axis[frozen], axis[end + 1] - extent)
      return current
    }
    element.scrollLeft = reveal(sheet.columns, target.s.c, target.e.c, sheet.frozen.c, element.scrollLeft, element.clientWidth - ROW_HEADER)
    element.scrollTop = reveal(sheet.rows, target.s.r, target.e.r, sheet.frozen.r, element.scrollTop, element.clientHeight - COLUMN_HEADER)
  }, [sheet, selection.focus, dragging])

  const pointAt = useCallback((x: number, y: number): CellAddress => {
    const element = scroll.current!
    const bounds = element.getBoundingClientRect()
    const localX = Math.max(0, Math.min(x - bounds.left - ROW_HEADER, element.clientWidth - ROW_HEADER - 1))
    const localY = Math.max(0, Math.min(y - bounds.top - COLUMN_HEADER, element.clientHeight - COLUMN_HEADER - 1))
    const c = spreadsheetIndex(sheet.columns, localX + (localX < frozenWidth ? 0 : element.scrollLeft))
    const r = spreadsheetIndex(sheet.rows, localY + (localY < frozenHeight ? 0 : element.scrollTop))
    return { r, c }
  }, [sheet, frozenWidth, frozenHeight])

  useEffect(() => {
    if (!dragging) return
    let frame = 0
    const tick = () => {
      const pointer = drag.current
      const element = scroll.current!
      if (!pointer) return
      const bounds = element.getBoundingClientRect()
      const dx = pointer.x < bounds.left + ROW_HEADER ? -18 : pointer.x > bounds.right - 20 ? 18 : 0
      const dy = pointer.y < bounds.top + COLUMN_HEADER ? -18 : pointer.y > bounds.bottom - 20 ? 18 : 0
      element.scrollLeft += dx
      element.scrollTop += dy
      const focus = pointAt(pointer.x, pointer.y)
      setSelection((previous) => previous.focus.r === focus.r && previous.focus.c === focus.c ? previous : { ...previous, focus })
      frame = requestAnimationFrame(tick)
    }
    frame = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(frame)
  }, [dragging, pointAt])

  const keyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    const edge = event.ctrlKey || event.metaKey
    if (edge && event.key.toLowerCase() === 'a') {
      event.preventDefault()
      setSelection({ anchor: { r: 0, c: 0 }, focus: { r: source.used.e.r, c: source.used.e.c } })
      return
    }
    const next = moveSpreadsheetSelection(sheet, selection, event.key, event.shiftKey, edge, Math.max(1, rows.length - 2))
    if (next) {
      event.preventDefault()
      setSelection(next)
    }
  }

  return <div className="spreadsheet-view">
    <div className="spreadsheet-formula-bar">
      <output className="spreadsheet-address" aria-label="Selected range">{address}</output>
      <span className="spreadsheet-fx" title={active?.f ? `Cached result: ${active.w ?? active.v ?? 'unavailable'}` : 'Cell value'} aria-hidden>fx</span>
      <div role="textbox" aria-readonly="true" aria-multiline="true" aria-label={active?.f ? 'Formula' : 'Cell value'} className="spreadsheet-value scrollbar-quiet">{fullValue}</div>
    </div>
    <div
      ref={scroll}
      role="grid"
      aria-label={sheet.name}
      aria-readonly="true"
      aria-rowcount={sheet.rows.length - 1}
      aria-colcount={sheet.columns.length - 1}
      aria-activedescendant={activeVisible ? `${id}-${activePane}-${utils.encode_cell(activeRange.s)}` : undefined}
      tabIndex={0}
      className="spreadsheet-scroll scrollbar-quiet"
      onScroll={(event) => {
        const element = event.currentTarget
        setViewport({ width: element.clientWidth, height: element.clientHeight, left: element.scrollLeft, top: element.scrollTop })
      }}
      onKeyDown={keyDown}
      onCopy={(event) => {
        event.preventDefault()
        event.clipboardData.setData('text/plain', selectionTSV(sheet, selection))
      }}
      onPointerDown={(event) => {
        if (event.button !== 0) return
        const bounds = event.currentTarget.getBoundingClientRect()
        if (event.clientX < bounds.left + ROW_HEADER || event.clientY < bounds.top + COLUMN_HEADER || event.clientX >= bounds.left + event.currentTarget.clientWidth || event.clientY >= bounds.top + event.currentTarget.clientHeight) return
        event.preventDefault()
        event.currentTarget.focus({ preventScroll: true })
        event.currentTarget.setPointerCapture(event.pointerId)
        const focus = pointAt(event.clientX, event.clientY)
        setSelection((previous) => ({ anchor: event.shiftKey ? previous.anchor : mergedRange(sheet, focus).s, focus }))
        drag.current = { x: event.clientX, y: event.clientY }
        setDragging(true)
      }}
      onPointerMove={(event) => {
        if (drag.current) drag.current = { x: event.clientX, y: event.clientY }
      }}
      onLostPointerCapture={() => {
        drag.current = null
        setDragging(false)
      }}
      onPointerUp={() => {
        drag.current = null
        setDragging(false)
      }}
    >
      <div style={{ width: Math.max(viewport.width, sheet.columns.at(-1)! + ROW_HEADER), height: Math.max(viewport.height, sheet.rows.at(-1)! + COLUMN_HEADER) }}>
        <div className="spreadsheet-viewport" style={{ width: viewport.width, height: viewport.height }}>
          {panes.map((pane, index) => pane.width > 0 && pane.height > 0 ? <Pane key={index} id={`${id}-${index}`} sheet={sheet} selection={selection} {...pane} /> : null)}
          {[{ indices: columns, offset: viewport.left, frozen: false }, { indices: frozenColumns, offset: 0, frozen: true }].map(({ indices, offset, frozen }) => <div key={String(frozen)} className="spreadsheet-column-headers" style={{ left: ROW_HEADER + (frozen ? 0 : frozenWidth), width: frozen ? frozenWidth : viewport.width - ROW_HEADER - frozenWidth }}>
            {indices.map((c) => <div key={c} role="columnheader" className={c >= range.s.c && c <= range.e.c ? 'is-selected' : ''} style={{ left: sheet.columns[c] - offset - (frozen ? 0 : frozenWidth), width: sheet.columns[c + 1] - sheet.columns[c], height: COLUMN_HEADER }}>{utils.encode_col(c)}</div>)}
          </div>)}
          {[{ indices: rows, offset: viewport.top, frozen: false }, { indices: frozenRows, offset: 0, frozen: true }].map(({ indices, offset, frozen }) => <div key={String(frozen)} className="spreadsheet-row-headers" style={{ top: COLUMN_HEADER + (frozen ? 0 : frozenHeight), height: frozen ? frozenHeight : viewport.height - COLUMN_HEADER - frozenHeight, width: ROW_HEADER }}>
            {indices.map((r) => <div key={r} role="rowheader" className={r >= range.s.r && r <= range.e.r ? 'is-selected' : ''} style={{ top: sheet.rows[r] - offset - (frozen ? 0 : frozenHeight), height: sheet.rows[r + 1] - sheet.rows[r], width: ROW_HEADER }}>{r + 1}</div>)}
          </div>)}
          <div className="spreadsheet-corner" style={{ width: ROW_HEADER, height: COLUMN_HEADER }} />
          {frozenWidth > 0 && <div className="spreadsheet-freeze-column" style={{ left: ROW_HEADER + frozenWidth }} />}
          {frozenHeight > 0 && <div className="spreadsheet-freeze-row" style={{ top: COLUMN_HEADER + frozenHeight }} />}
        </div>
      </div>
    </div>
  </div>
}
