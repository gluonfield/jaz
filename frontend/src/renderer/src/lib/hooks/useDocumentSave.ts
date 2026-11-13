import type { Editor } from '@tiptap/core'
import { useEffect, useRef, useState } from 'react'
import { ApiError } from '@/lib/api/response'
import { savedMarkdown } from '@/lib/savedMarkdown'

export type SaveState = 'saved' | 'saving' | 'conflict' | 'failed'
export const SAVE_LABEL: Record<SaveState, string> = { saved: '', saving: 'Saving…', conflict: 'Not saved', failed: "Couldn't save" }

export interface DocumentVersion {
  content: string
  version: string
}

// useDocumentSave saves a Markdown editor's document over the file it was
// opened from. Only the person's edits are saved, each over the version read,
// so another writer's changes meanwhile are never overwritten unseen: write
// fails with the conflict status when the file is no longer that version, and
// the person then chooses between the two. read and write are the ones given
// when the document opened.
export function useDocumentSave(
  file: DocumentVersion,
  { read, write, conflict, delay }: {
    read: () => Promise<DocumentVersion>
    write: (content: string, version: string) => Promise<DocumentVersion>
    conflict: number
    delay: number
  },
) {
  const version = useRef(file.version)
  const source = useRef(file.content)
  const edited = useRef(false)
  const saving = useRef<Promise<void>>(Promise.resolve())
  const timer = useRef(0)
  const view = useRef<Editor | null>(null)
  const [state, setState] = useState<SaveState>('saved')
  const status = useRef(state)
  const [actions] = useState(() => {
    const set = (next: SaveState) => {
      status.current = next
      setState(next)
    }
    const adopt = (editor: Editor, current: DocumentVersion) => {
      version.current = current.version
      source.current = current.content
      edited.current = false
      editor.commands.setContent(current.content, { contentType: 'markdown', emitUpdate: false })
      set('saved')
    }
    // Saves are chained so each goes over the version the one before it wrote,
    // and wait for the person's choice once one has been refused.
    const save = () => {
      window.clearTimeout(timer.current)
      saving.current = saving.current.then(async () => {
        if (!edited.current || !view.current || status.current === 'conflict') return
        edited.current = false
        set('saving')
        try {
          const saved = await write(savedMarkdown(view.current, source.current), version.current)
          version.current = saved.version
          source.current = saved.content
          set(edited.current ? 'saving' : 'saved')
        } catch (error) {
          edited.current = true
          set(error instanceof ApiError && error.status === conflict ? 'conflict' : 'failed')
        }
      })
      return saving.current
    }
    return {
      save,
      // Development's StrictMode builds the editor twice, and only the one
      // being typed in is known to be on screen, so that is the one saved.
      touch: (editor: Editor) => {
        view.current = editor
        edited.current = true
        window.clearTimeout(timer.current)
        timer.current = window.setTimeout(() => void save(), delay)
      },
      // follow shows a version written elsewhere once pending saves are done,
      // unless the person has edits it would discard or one of those saves
      // has since replaced what was read.
      follow: (editor: Editor, current: DocumentVersion) => {
        const over = version.current
        saving.current = saving.current.then(() => {
          if (!edited.current && over === version.current && current.version !== over) adopt(editor, current)
        })
      },
      takeTheirs: async () => adopt(view.current!, await read()),
      keepMine: async () => {
        version.current = (await read()).version
        set('saving')
        await save()
      },
    }
  })
  useEffect(() => () => void actions.save(), [actions])
  return { state, edited, ...actions }
}
