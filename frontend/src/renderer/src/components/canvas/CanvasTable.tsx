import type { NodeProps } from '@xyflow/react'
import { PanelTop, Plus } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { MessageMarkdown } from '@/components/session/MessageMarkdown'
import { FitContent } from '@/components/canvas/FitContent'
import type { BoardNode } from './board'
import { useCanvas } from './canvasContext'
import { Flash, Handles, Resizer } from './CanvasNodes'
import { SizeMenu } from './CanvasTextToolbar'
import { ItemToolbar, ToolButton } from './CanvasToolbar'
import { editTable, lookOf, tableRows, type TableEdit } from './format'

type Cell = [row: number, column: number]

export function TableNode({ id, data, selected }: NodeProps<BoardNode>) {
  const { commit, connectSource } = useCanvas()
  const rows = tableRows(data.item)
  const header = data.item.header !== false
  const size = lookOf(data.item).size
  const [active, setActive] = useState<Cell | null>(null)
  const [typing, setTyping] = useState(false)
  const change = (edit: TableEdit) => commit({ nodes: [{ id, rows: editTable(rows, edit) }] })
  const at: Cell = active && active[0] < rows.length && active[1] < rows[0].length ? active : [rows.length - 1, rows[0].length - 1]
  const finish = (cell: Cell, text: string, step: number) => {
    if (text !== rows[cell[0]][cell[1]]) change({ cell, text })
    const next = cell[0] * rows[0].length + cell[1] + step
    const moved = step !== 0 && next >= 0 && next < rows.length * rows[0].length
    setTyping(moved)
    if (moved) setActive([Math.floor(next / rows[0].length), next % rows[0].length])
  }
  return (
    <>
      <Resizer item={data.item} selected={selected && !typing} minWidth={160} />
      {selected ? <ItemToolbar label="Table" item={data.item}>
        <SizeMenu item={data.item} />
        <ToolButton label="Heading row" on={data.item.header !== false} onClick={() => commit({ nodes: [{ id, header: data.item.header === false ? null : false }] })}><PanelTop size={16} /></ToolButton>
        <span className="canvas-toolbar-rule" />
        <ToolButton label="Add row below" onClick={() => change({ addRow: at[0] + 1 })}><span className="canvas-toolbar-word">+ Row</span></ToolButton>
        <ToolButton label="Add column right" onClick={() => change({ addColumn: at[1] + 1 })}><span className="canvas-toolbar-word">+ Column</span></ToolButton>
        <ToolButton label="Remove row" onClick={() => change({ removeRow: at[0] })}><span className="canvas-toolbar-word">− Row</span></ToolButton>
        <ToolButton label="Remove column" onClick={() => change({ removeColumn: at[1] })}><span className="canvas-toolbar-word">− Column</span></ToolButton>
      </ItemToolbar> : null}
      <Handles />
      <div className={`canvas-card canvas-table-card${size ? ` canvas-size-${size}` : ''}${connectSource === id ? ' canvas-connect-source' : ''}`}>
        <div className="canvas-table-body">
          <FitContent>
            <table className="canvas-table">
              <tbody>
                {rows.map((row, r) => (
                  <tr key={r}>
                    {row.map((text, c) => {
                      const Tag = header && r === 0 ? 'th' : 'td'
                      const on = selected && active?.[0] === r && active[1] === c
                      return (
                        <Tag
                          key={c}
                          className={on ? 'on' : ''}
                          onClick={() => setActive([r, c])}
                          onDoubleClick={() => {
                            setActive([r, c])
                            setTyping(data.item.locked !== true)
                          }}
                        >
                          {on && typing ? <CellEditor key={`${r}:${c}`} initial={text} onDone={(value, step) => finish([r, c], value, step)} /> : <MessageMarkdown text={text} />}
                        </Tag>
                      )
                    })}
                  </tr>
                ))}
              </tbody>
            </table>
          </FitContent>
        </div>
        <Flash id={id} />
      </div>
      {selected ? (
        <>
          <button type="button" className="canvas-table-add row nodrag" aria-label="Add row" onClick={() => change({ addRow: rows.length })}><Plus size={12} /></button>
          <button type="button" className="canvas-table-add column nodrag" aria-label="Add column" onClick={() => change({ addColumn: rows[0].length })}><Plus size={12} /></button>
        </>
      ) : null}
    </>
  )
}

// Enter keeps the cell, Tab keeps it and moves on, Escape leaves it as it was.
function CellEditor({ initial, onDone }: { initial: string; onDone: (value: string, step: number) => void }) {
  const [value, setValue] = useState(initial)
  const field = useRef<HTMLTextAreaElement>(null)
  const done = useRef(false)
  useEffect(() => {
    field.current!.focus()
    field.current!.select()
  }, [])
  const finish = (text: string, step: number) => {
    if (done.current) return
    done.current = true
    onDone(text, step)
  }
  return (
    <textarea
      ref={field}
      className="canvas-text-editor nodrag nopan"
      rows={1}
      value={value}
      onChange={(event) => setValue(event.target.value)}
      onBlur={() => finish(value, 0)}
      onKeyDown={(event) => {
        if (event.key === 'Escape') finish(initial, 0)
        if (event.key === 'Enter' && !event.shiftKey) {
          event.preventDefault()
          finish(value, 0)
        }
        if (event.key === 'Tab') {
          event.preventDefault()
          finish(value, event.shiftKey ? -1 : 1)
        }
      }}
    />
  )
}
