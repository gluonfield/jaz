import type { Editor } from '@tiptap/core'
import { useEditorState } from '@tiptap/react'
import { TableOfContents } from 'lucide-react'
import { useState } from 'react'
import { IconButton } from '@/components/ui/IconButton'
import { MenuRow, Popover } from '@/components/ui/Popover'

type Heading = { pos: number; level: number; text: string }

function headings(editor: Editor): Heading[] {
  const found: Heading[] = []
  editor.state.doc.forEach((node, pos) => {
    if (node.type.name === 'heading') found.push({ pos, level: node.attrs.level as number, text: node.textContent })
  })
  return found
}

// The document's headings as a list to jump by, once there is more than a title.
export function DocumentOutline({ editor }: { editor: Editor }) {
  const [open, setOpen] = useState(false)
  const outline = useEditorState({ editor, selector: ({ editor }) => headings(editor) })
  if (outline.length < 2) return null
  return (
    <Popover
      open={open}
      onClose={() => setOpen(false)}
      placement="below"
      align="end"
      trigger={
        <IconButton size="sm" aria-label="Outline" title="Outline" aria-expanded={open} onClick={() => setOpen(!open)}>
          <TableOfContents size={15} />
        </IconButton>
      }
    >
      <div className="scrollbar-quiet max-h-80 w-64 overflow-auto">
        {outline.map((heading) => (
          <MenuRow
            key={heading.pos}
            role="menuitem"
            onClick={() => {
              setOpen(false)
              ;(editor.view.nodeDOM(heading.pos) as HTMLElement).scrollIntoView({ block: 'start', behavior: 'smooth' })
            }}
          >
            <span style={{ paddingLeft: (heading.level - 1) * 12 }}>{heading.text}</span>
          </MenuRow>
        ))}
      </div>
    </Popover>
  )
}
