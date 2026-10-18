import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useBlocker } from '@tanstack/react-router'
import { EditorContent, useEditor, useEditorState, type Editor } from '@tiptap/react'
import { Bold, Italic, List, ListOrdered, X } from 'lucide-react'
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { Button } from '@/components/ui/Button'
import { IconButton } from '@/components/ui/IconButton'
import { botFilesQuery, saveBotFile } from '@/lib/api/bots'
import { ApiError } from '@/lib/api/client'
import type { Bot, BotFile, BotFileName } from '@/lib/api/types'
import { isMacDesktop } from '@/lib/clientRuntime'
import { consumeEscapeKey, createEscapeLayerID, isTopEscapeLayer, pushEscapeLayer } from '@/lib/dom/escapeLayers'
import { keys } from '@/lib/query/keys'
import { markdownKit } from './markdownKit'

type SaveState = 'saved' | 'saving' | 'conflict' | 'failed'

// A bot's AGENTS.md or MEMORY.md, edited as a document over the whole window.
// Only the person's edits are saved, each over the version read, so the bot's
// own writes meanwhile are never overwritten unseen.
export function BotFileEditor({ bot, name, onClose }: { bot: Bot; name: BotFileName; onClose: () => void }) {
  const file = useQuery(botFilesQuery(bot.id)).data?.find((item) => item.name === name)
  return createPortal(
    <div role="dialog" aria-modal="true" aria-label={name} className="fixed inset-0 z-modal flex flex-col bg-bg">
      {file ? <FileDocument key={`${bot.id}:${name}`} bot={bot} file={file} onClose={onClose} /> : <Header name={name} onClose={onClose} />}
    </div>,
    document.body,
  )
}

function FileDocument({ bot, file, onClose }: { bot: Bot; file: BotFile; onClose: () => void }) {
  const queryClient = useQueryClient()
  const version = useRef(file.version)
  const edited = useRef(false)
  const saving = useRef<Promise<void>>(Promise.resolve())
  const timer = useRef(0)
  const view = useRef<Editor | null>(null)
  const [state, setState] = useState<SaveState>('saved')
  // Saves are chained so each goes over the version the one before it wrote.
  const [save] = useState(() => () => {
    window.clearTimeout(timer.current)
    saving.current = saving.current.then(async () => {
      if (!edited.current || !view.current) return
      edited.current = false
      setState('saving')
      try {
        const saved = await saveBotFile(bot.id, file.name, view.current.getMarkdown(), version.current)
        version.current = saved.version
        queryClient.setQueryData(keys.botFiles(bot.id), (files: BotFile[] | undefined) => files?.map((item) => (item.name === saved.name ? saved : item)))
        void queryClient.invalidateQueries({ queryKey: keys.botActivity(bot.id) })
        setState(edited.current ? 'saving' : 'saved')
      } catch (error) {
        edited.current = true
        setState(error instanceof ApiError && error.status === 409 ? 'conflict' : 'failed')
      }
    })
    return saving.current
  })
  const editor = useEditor({
    extensions: markdownKit,
    content: file.content,
    contentType: 'markdown',
    autofocus: 'end',
    editorProps: { attributes: { class: 'editor-prose min-h-full outline-none', 'aria-label': file.name } },
    onCreate: ({ editor }) => {
      view.current = editor
    },
    onUpdate: () => {
      edited.current = true
      window.clearTimeout(timer.current)
      timer.current = window.setTimeout(() => void save(), 800)
    },
  })

  const latest = async () => {
    const files = await queryClient.fetchQuery({ ...botFilesQuery(bot.id), staleTime: 0 })
    return files.find((item) => item.name === file.name)!
  }
  const takeTheirs = async () => {
    const current = await latest()
    version.current = current.version
    edited.current = false
    editor?.commands.setContent(current.content, { contentType: 'markdown', emitUpdate: false })
    setState('saved')
  }
  const keepMine = async () => {
    version.current = (await latest()).version
    await save()
  }
  // Closing saves first and stays open on an edit that could not be saved.
  const close = async () => {
    if (state !== 'conflict') await save()
    if (edited.current && !window.confirm(`Discard your edits to ${file.name}?`)) return
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
  useEffect(() => () => void save(), [save])

  return (
    <>
      <Header name={file.name} onClose={() => void close()} state={state}>
        {editor ? <Toolbar editor={editor} /> : null}
      </Header>
      {state === 'conflict' ? (
        <div className="flex items-center justify-center gap-3 border-b border-border bg-surface px-4 py-2 text-[13px] text-ink-2">
          <span>
            {bot.name} changed {file.name} while you were editing.
          </span>
          <Button size="sm" variant="ghost" onClick={() => void takeTheirs()}>
            Use theirs
          </Button>
          <Button size="sm" variant="secondary" onClick={() => void keepMine()}>
            Keep mine
          </Button>
        </div>
      ) : null}
      <div className="min-h-0 flex-1 overflow-y-auto" onClick={() => editor?.commands.focus()}>
        <div className="mx-auto max-w-[720px] px-8 py-12">
          <EditorContent editor={editor} />
        </div>
      </div>
    </>
  )
}

const SAVE_LABEL: Record<SaveState, string> = { saved: '', saving: 'Saving…', conflict: 'Not saved', failed: "Couldn't save" }

function Header({ name, state = 'saved', onClose, children }: { name: string; state?: SaveState; onClose: () => void; children?: ReactNode }) {
  useEscape(onClose)
  return (
    <header
      className="titlebar-drag grid h-[52px] shrink-0 grid-cols-[1fr_auto_1fr] items-center gap-3 border-b border-border pr-3"
      style={{ paddingLeft: isMacDesktop ? 84 : 16 }}
    >
      <span className="truncate text-[14px] font-semibold text-ink">{name}</span>
      <div className="flex items-center gap-0.5">{children}</div>
      <div className="flex items-center justify-end gap-2">
        <span role="status" className={`text-[12px] ${state === 'saving' ? 'text-ink-3' : 'text-danger'}`}>
          {SAVE_LABEL[state]}
        </span>
        <IconButton size="sm" aria-label="Close" title="Close" onClick={onClose}>
          <X size={16} />
        </IconButton>
      </div>
    </header>
  )
}

function useEscape(onEscape: () => void) {
  const latest = useRef(onEscape)
  latest.current = onEscape
  useEffect(() => {
    const id = createEscapeLayerID('bot-file')
    const unregister = pushEscapeLayer(id)
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || !isTopEscapeLayer(id)) return
      consumeEscapeKey(event)
      latest.current()
    }
    document.addEventListener('keydown', onKey, true)
    return () => {
      document.removeEventListener('keydown', onKey, true)
      unregister()
    }
  }, [])
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
