import type { Editor } from '@tiptap/core'
import { TextSelection } from '@tiptap/pm/state'
import { useEffect, useState } from 'react'
import { ContextMenu } from '@/components/ui/Popover'
import { CommandRows } from './CommandRows'
import { tableEdits } from './documentCommands'

// A right-click in a table cell offers what can be done to its row, its
// column and the table.
export function TableMenu({ editor }: { editor: Editor }) {
  const [point, setPoint] = useState<{ x: number; y: number } | null>(null)
  useEffect(() => {
    const { view } = editor
    const onMenu = (event: MouseEvent) => {
      if (!(event.target as Element).closest('td, th')) return
      event.preventDefault()
      const at = view.posAtCoords({ left: event.clientX, top: event.clientY })
      if (at && view.state.selection.empty) view.dispatch(view.state.tr.setSelection(TextSelection.near(view.state.doc.resolve(at.pos))))
      setPoint({ x: event.clientX, y: event.clientY })
    }
    view.dom.addEventListener('contextmenu', onMenu)
    return () => view.dom.removeEventListener('contextmenu', onMenu)
  }, [editor])
  if (!point) return null
  return (
    <ContextMenu point={point} onClose={() => setPoint(null)}>
      <CommandRows
        groups={tableEdits}
        onPick={(command) => {
          setPoint(null)
          command.run(editor.chain().focus()).run()
        }}
      />
    </ContextMenu>
  )
}
