import type { Editor } from '@tiptap/core'
import { TextSelection, type EditorState } from '@tiptap/pm/state'
import { useEditorState } from '@tiptap/react'
import { BubbleMenu } from '@tiptap/react/menus'
import { Bold, Code, Italic, Link, Strikethrough, type LucideIcon } from 'lucide-react'
import { useState } from 'react'
import { IconButton } from '@/components/ui/IconButton'

const MARKS: [mark: string, label: string, icon: LucideIcon][] = [
  ['bold', 'Bold', Bold],
  ['italic', 'Italic', Italic],
  ['strike', 'Strikethrough', Strikethrough],
  ['code', 'Code', Code],
]

const selectsText = ({ state }: { state: EditorState }) => state.selection instanceof TextSelection && !state.selection.empty

// The marks for the words selected, shown beside them.
export function SelectionMenu({ editor }: { editor: Editor }) {
  const at = useEditorState({
    editor,
    selector: ({ editor }) => ({ range: `${editor.state.selection.from}:${editor.state.selection.to}`, marks: MARKS.filter(([mark]) => editor.isActive(mark)).map(([mark]) => mark), link: (editor.getAttributes('link').href as string | undefined) ?? null }),
  })
  return (
    <BubbleMenu editor={editor} shouldShow={selectsText} className="z-(--z-dropdown) flex items-center gap-0.5 rounded-full bg-surface p-1 shadow-lg ring-1 ring-border">
      <Marks key={at.range} editor={editor} marks={at.marks} link={at.link} />
    </BubbleMenu>
  )
}

function Marks({ editor, marks, link }: { editor: Editor; marks: string[]; link: string | null }) {
  const [linking, setLinking] = useState(false)
  if (linking) {
    return (
      <form
        onSubmit={(event) => {
          event.preventDefault()
          const href = new FormData(event.currentTarget).get('href') as string
          const chain = editor.chain().focus().extendMarkRange('link')
          void (href.trim() ? chain.setLink({ href: href.trim() }) : chain.unsetLink()).run()
          setLinking(false)
        }}
      >
        <input
          name="href"
          autoFocus
          defaultValue={link ?? ''}
          placeholder="Paste a link"
          aria-label="Link"
          className="h-8 w-64 bg-transparent px-3 text-[13px] outline-none"
          onKeyDown={(event) => {
            if (event.key !== 'Escape') return
            setLinking(false)
            editor.commands.focus()
          }}
        />
      </form>
    )
  }
  return (
    <>
      {MARKS.map(([mark, label, Icon]) => (
        <IconButton
          key={mark}
          size="md"
          aria-label={label}
          title={label}
          aria-pressed={marks.includes(mark)}
          className={marks.includes(mark) ? 'bg-surface-2 text-ink' : ''}
          onMouseDown={(event) => event.preventDefault()}
          onClick={() => editor.chain().focus().toggleMark(mark).run()}
        >
          <Icon size={15} />
        </IconButton>
      ))}
      <IconButton size="md" aria-label="Link" title="Link" aria-pressed={link !== null} className={link !== null ? 'bg-surface-2 text-ink' : ''} onClick={() => setLinking(true)}>
        <Link size={15} />
      </IconButton>
    </>
  )
}
