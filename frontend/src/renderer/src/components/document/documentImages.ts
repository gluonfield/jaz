import { Extension, type Editor } from '@tiptap/core'
import { Plugin } from '@tiptap/pm/state'
import { clipboardFiles } from '@/components/ui/fileTransfer'

declare module '@tiptap/core' {
  interface Commands<ReturnType> {
    documentImages: { pickImage: () => ReturnType }
  }
}

// SaveImage keeps a picture at a path relative to the document and tells
// whether it was kept.
export type SaveImage = (path: string, image: File) => Promise<boolean>

// Pictures a person picks or pastes go into an images folder beside the
// document, and each appears in the text once it has been kept there.
export function documentImages(save: SaveImage) {
  const add = (editor: Editor, files: File[]) => {
    const images = files.filter((file) => file.type.startsWith('image/'))
    for (const image of images) {
      const path = `images/${crypto.randomUUID().slice(0, 8)}-${image.name.replace(/[^\w.-]+/g, '-')}`
      const source = `![${image.name.replace(/\.\w+$/, '').replace(/[[\]]/g, '')}](${path})`
      void save(path, image).then((kept) => kept && !editor.isDestroyed && editor.commands.insertContent({ type: 'sourceInline', attrs: { source } }))
    }
    return images.length > 0
  }
  return Extension.create({
    name: 'documentImages',
    addCommands: () => ({
      pickImage:
        () =>
        ({ editor, dispatch }) => {
          if (!dispatch) return true
          const picker = Object.assign(document.createElement('input'), { type: 'file', accept: 'image/*', multiple: true })
          picker.onchange = () => add(editor, [...(picker.files ?? [])])
          picker.click()
          return true
        },
    }),
    addProseMirrorPlugins() {
      const paste = (data: DataTransfer | null) => data !== null && !data.types.includes('text/plain') && add(this.editor, clipboardFiles(data))
      return [new Plugin({ props: { handlePaste: (_view, event) => paste(event.clipboardData) } })]
    },
  })
}
