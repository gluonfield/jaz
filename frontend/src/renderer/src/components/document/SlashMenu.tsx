import type { Editor } from '@tiptap/core'
import { useEditorState } from '@tiptap/react'
import { useEffect, useMemo, useState } from 'react'
import { ContextMenu } from '@/components/ui/Popover'
import { CommandRows } from './CommandRows'
import { matchesQuery, slashQuery, type DocumentCommand } from './documentCommands'

// The height of a menu row, to tell whether the menu fits below the caret.
const ROW = 28

// The menu a "/" opens at the caret: typing narrows it, the arrows and Enter
// pick from it, and the pick takes the place of what was typed.
export function SlashMenu({ editor, groups }: { editor: Editor; groups: DocumentCommand[][] }) {
  const slash = useEditorState({ editor, selector: ({ editor }) => (editor.isFocused ? slashQuery(editor.state) : null) })
  const [closedAt, setClosedAt] = useState(-1)
  const [picked, setPicked] = useState({ query: '', index: 0 })
  const query = slash?.query ?? ''
  const shown = useMemo(() => groups.map((group) => group.filter((command) => matchesQuery(command, query))).filter((group) => group.length), [groups, query])
  const commands = shown.flat()
  const from = slash && slash.from !== closedAt && commands.length ? slash.from : -1
  const index = picked.query === query ? Math.min(picked.index, commands.length - 1) : 0
  const point = useMemo(() => {
    if (from < 0) return null
    const caret = editor.view.coordsAtPos(from)
    const height = commands.length * ROW + shown.length * 9 + 12
    return { x: caret.left, y: caret.bottom + 6 + height > innerHeight ? caret.top - 6 - height : caret.bottom + 6 }
  }, [editor, from, commands.length, shown.length])
  const choose = (command: DocumentCommand) => command.run(editor.chain().focus().deleteRange(slash!)).run()
  useEffect(() => {
    if (!slash) setClosedAt(-1)
  }, [slash])
  useEffect(() => {
    if (!point) return
    const onKey = (event: KeyboardEvent) => {
      const step = event.key === 'ArrowDown' ? 1 : event.key === 'ArrowUp' ? -1 : 0
      if (!step && event.key !== 'Enter' && event.key !== 'Tab') return
      event.preventDefault()
      event.stopPropagation()
      if (step) setPicked({ query, index: (index + step + commands.length) % commands.length })
      else choose(commands[index])
    }
    document.addEventListener('keydown', onKey, true)
    return () => document.removeEventListener('keydown', onKey, true)
  })
  if (!point) return null
  return (
    <ContextMenu point={point} onClose={() => setClosedAt(from)}>
      <div role="menu" aria-label="Insert" onMouseDown={(event) => event.preventDefault()}>
        <CommandRows groups={shown} active={commands[index]} onPick={choose} />
      </div>
    </ContextMenu>
  )
}
