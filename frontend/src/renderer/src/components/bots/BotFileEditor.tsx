import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useBlocker } from '@tanstack/react-router'
import { EditorContent, useEditor, useEditorState, type Editor } from '@tiptap/react'
import { Bold, Italic, List, ListOrdered } from 'lucide-react'
import type { ReactNode } from 'react'
import { Button } from '@/components/ui/Button'
import { Modal } from '@/components/ui/Modal'
import { botFilesQuery, saveBotFile } from '@/lib/api/bots'
import type { Bot, BotFile, BotFileName } from '@/lib/api/types'
import { BOT_FILE_LABELS } from '@/lib/bots'
import { keys } from '@/lib/query/keys'
import { SAVE_LABEL, useDocumentSave } from '@/lib/hooks/useDocumentSave'
import { markdownKit } from './markdownKit'

// A bot's BOTS.md or MEMORY.md, edited as a document in a modal. Only the
// person's edits are saved, each over the version read, so the bot's own
// writes meanwhile are never overwritten unseen.
export function BotFileEditor({ bot, name, onClose }: { bot: Bot; name: BotFileName; onClose: () => void }) {
  const file = useQuery(botFilesQuery(bot.id)).data?.find((item) => item.name === name)
  return file ? <FileDocument key={`${bot.id}:${name}`} bot={bot} file={file} onClose={onClose} /> : null
}

function FileDocument({ bot, file, onClose }: { bot: Bot; file: BotFile; onClose: () => void }) {
  const label = BOT_FILE_LABELS[file.name]
  const queryClient = useQueryClient()
  const { state, edited, save, touch, takeTheirs, keepMine } = useDocumentSave(file, {
    read: async () => (await queryClient.fetchQuery({ ...botFilesQuery(bot.id), staleTime: 0 })).find((item) => item.name === file.name)!,
    write: async (content, version) => {
      const saved = await saveBotFile(bot.id, file.name, content, version)
      queryClient.setQueryData(keys.botFiles(bot.id), (files: BotFile[] | undefined) => files?.map((item) => (item.name === saved.name ? saved : item)))
      void queryClient.invalidateQueries({ queryKey: keys.botActivity(bot.id) })
      return saved
    },
    conflict: 409,
    delay: 800,
  })
  const editor = useEditor({
    extensions: markdownKit,
    content: file.content,
    contentType: 'markdown',
    autofocus: 'end',
    editorProps: { attributes: { class: 'editor-prose outline-none', 'aria-label': label } },
    onUpdate: ({ editor }) => touch(editor),
  })

  // Closing saves first and stays open on an edit that could not be saved.
  const close = async () => {
    await save()
    if (edited.current && !window.confirm(`Discard your edits to ${label}?`)) return
    onClose()
  }

  // Leaving for another page saves the edit first and stays, draft and all,
  // when it cannot be saved; any other unmount still saves what it can.
  useBlocker({
    shouldBlockFn: async () => {
      await save()
      return edited.current
    },
    enableBeforeUnload: false,
  })

  // A fixed height keeps the modal from resizing as the document grows or the
  // conflict choice appears.
  return (
    <Modal
      open
      onClose={() => void close()}
      title={label}
      size="wide"
      className="h-[min(720px,calc(100dvh-3rem))]"
      // The negative margin keeps the toolbar on the title's line.
      headerAccessory={
        <div className="-my-1.5 flex items-center gap-2">
          <span role="status" className={`text-[12px] ${state === 'saving' ? 'text-ink-3' : 'text-danger'}`}>
            {SAVE_LABEL[state]}
          </span>
          {editor ? (
            <span className="flex items-center gap-0.5">
              <Toolbar editor={editor} />
            </span>
          ) : null}
        </div>
      }
      footer={
        state === 'conflict' ? (
          <>
            <span className="text-[13px] text-ink-2">
              {bot.name} changed {label} while you were editing.
            </span>
            <span className="flex shrink-0 gap-2">
              <Button size="sm" variant="ghost" onClick={() => void takeTheirs()}>
                Use theirs
              </Button>
              <Button size="sm" variant="secondary" onClick={() => void keepMine()}>
                Keep mine
              </Button>
            </span>
          </>
        ) : undefined
      }
    >
      <div className="min-h-full cursor-text" onClick={() => editor?.commands.focus()}>
        <EditorContent editor={editor} />
      </div>
    </Modal>
  )
}

function Toolbar({ editor }: { editor: Editor }) {
  const active = useEditorState({
    editor,
    selector: ({ editor }) => ({
      bold: editor.isActive('bold'),
      italic: editor.isActive('italic'),
      h1: editor.isActive('heading', { level: 1 }),
      h2: editor.isActive('heading', { level: 2 }),
      h3: editor.isActive('heading', { level: 3 }),
      bullets: editor.isActive('bulletList'),
      numbers: editor.isActive('orderedList'),
    }),
  })
  const chain = () => editor.chain().focus()
  return (
    <>
      <Tool label="Bold" on={active.bold} run={() => chain().toggleBold().run()}>
        <Bold size={15} />
      </Tool>
      <Tool label="Italic" on={active.italic} run={() => chain().toggleItalic().run()}>
        <Italic size={15} />
      </Tool>
      <span className="mx-1.5 h-4 w-px bg-border" aria-hidden />
      {([1, 2, 3] as const).map((level) => (
        <Tool key={level} label={`Heading ${level}`} on={active[`h${level}`]} run={() => chain().toggleHeading({ level }).run()}>
          <span className="text-[12px] font-semibold">H{level}</span>
        </Tool>
      ))}
      <span className="mx-1.5 h-4 w-px bg-border" aria-hidden />
      <Tool label="Bulleted list" on={active.bullets} run={() => chain().toggleBulletList().run()}>
        <List size={16} />
      </Tool>
      <Tool label="Numbered list" on={active.numbers} run={() => chain().toggleOrderedList().run()}>
        <ListOrdered size={16} />
      </Tool>
    </>
  )
}

function Tool({ label, on, run, children }: { label: string; on: boolean; run: () => void; children: ReactNode }) {
  return (
    <button
      type="button"
      aria-label={label}
      aria-pressed={on}
      title={label}
      onMouseDown={(event) => event.preventDefault()}
      onClick={run}
      className={`grid size-8 cursor-pointer place-items-center rounded-md transition-colors duration-150 hover:bg-list-hover hover:text-ink ${on ? 'bg-list-active text-ink' : 'text-ink-3'}`}
    >
      {children}
    </button>
  )
}
