import type { Editor } from '@tiptap/core'
import { useEditorState } from '@tiptap/react'
import { Ellipsis, Type, type LucideIcon } from 'lucide-react'
import { useState } from 'react'
import { IconButton } from '@/components/ui/IconButton'
import { Popover } from '@/components/ui/Popover'
import { CommandRows } from './CommandRows'
import { tableEdits, type documentCommands, type DocumentCommand } from './documentCommands'

// The bar that stays at the foot of a document: the style of the line the
// caret is on, and the things that can be put there. Inside a table its table
// button edits that table instead of adding another.
export function DocumentToolbar({ editor, commands }: { editor: Editor; commands: ReturnType<typeof documentCommands> }) {
  const [open, setOpen] = useState('')
  const at = useEditorState({
    editor,
    selector: ({ editor }) => ({ style: commands.styles.findLast((style) => style.active?.(editor))?.label, table: editor.isActive('table'), checklist: editor.isActive('taskList') }),
  })
  const run = (command: DocumentCommand) => {
    setOpen('')
    command.run(editor.chain().focus()).run()
  }
  const menu = (label: string, Icon: LucideIcon, groups: DocumentCommand[][], selected?: string) => (
    <Popover
      open={open === label}
      onClose={() => setOpen('')}
      trigger={
        <IconButton size="md" aria-label={label} title={label} aria-expanded={open === label} onClick={() => setOpen(open === label ? '' : label)}>
          <Icon size={16} />
        </IconButton>
      }
    >
      <CommandRows groups={groups} selected={selected} onPick={run} />
    </Popover>
  )
  const button = (command: DocumentCommand, pressed?: boolean) => (
    <IconButton size="md" aria-label={command.label} title={command.label} aria-pressed={pressed} className={pressed ? 'bg-surface-2 text-ink' : ''} onClick={() => run(command)}>
      <command.icon size={16} />
    </IconButton>
  )
  return (
    <div className="pointer-events-none absolute inset-x-0 bottom-4 flex justify-center">
      <div role="toolbar" aria-label="Document" className="pointer-events-auto flex items-center gap-0.5 rounded-full bg-surface p-1 shadow-lg ring-1 ring-border" onMouseDown={(event) => event.preventDefault()}>
        {menu('Text style', Type, [commands.styles], at.style)}
        {at.table ? menu('Edit table', commands.table.icon, tableEdits) : button(commands.table)}
        {button(commands.image)}
        {button(commands.checklist, at.checklist)}
        {menu('More', Ellipsis, commands.more)}
      </div>
    </div>
  )
}
