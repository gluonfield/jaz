import { BrowserWindow, Menu, clipboard, shell, type MenuItemConstructorOptions, type WebContents } from 'electron'
import { isPreviewURL } from '@shared/preview'

function imageCopyPath(value: string, backendOrigin: string): string {
  if (!value) return ''
  const source = new URL(value)
  if (source.origin !== backendOrigin) return value
  const path = /^\/v1\/sessions\/[^/]+\/file$/.test(source.pathname) ? source.searchParams.get('path') : null
  if (path !== null) return path
  source.searchParams.delete('ticket')
  source.searchParams.delete('image')
  return source.href
}

// Electron ships no default right-click menu; build the link and standard text
// actions wherever the click lands on an image, link, editable field, or selection.
export function attachContextMenu(contents: WebContents, options: {
  canOpenPreviewURL: (contents: WebContents) => boolean
  openPreviewURL: (url: string, contents: WebContents) => boolean
  backendOrigin: () => string
}): void {
  contents.on('context-menu', (_event, params) => {
    const linkURL = params.linkURL.trim()
    const previewLinkURL = isPreviewURL(linkURL) ? linkURL : ''
    const canOpenSideBrowser = previewLinkURL !== '' && options.canOpenPreviewURL(contents)
    const editable = params.isEditable
    const selection = params.selectionText.trim()
    const image = params.mediaType === 'image'
    if (!image && !previewLinkURL && !editable && selection === '') return

    const items: MenuItemConstructorOptions[] = []
    if (image) {
      const path = imageCopyPath(params.srcURL, options.backendOrigin())
      items.push(
        {
          label: 'Copy Image',
          enabled: params.hasImageContents,
          click: () => contents.copyImageAt(params.x, params.y),
        },
        {
          label: 'Copy Path',
          enabled: path !== '',
          click: () => clipboard.writeText(path),
        },
      )
      if (previewLinkURL || editable || selection !== '') items.push({ type: 'separator' })
    }
    if (previewLinkURL) {
      if (canOpenSideBrowser) {
        items.push({
          label: 'Open in Side Browser',
          click: () => options.openPreviewURL(previewLinkURL, contents),
        })
      }
      items.push(
        {
          label: 'Open in Browser',
          click: () => shell.openExternal(previewLinkURL),
        },
      )
      if (editable || selection !== '') items.push({ type: 'separator' })
    }
    if (editable && params.misspelledWord) {
      for (const suggestion of params.dictionarySuggestions.slice(0, 5)) {
        items.push({ label: suggestion, click: () => contents.replaceMisspelling(suggestion) })
      }
      items.push(
        {
          label: 'Add to Dictionary',
          click: () => contents.session.addWordToSpellCheckerDictionary(params.misspelledWord),
        },
        { type: 'separator' },
      )
    }
    if (selection !== '') {
      const preview = selection.length > 30 ? `${selection.slice(0, 30)}…` : selection
      if (process.platform === 'darwin') {
        items.push({
          label: `Look Up “${preview}”`,
          click: () => contents.showDefinitionForSelection(),
        })
      }
      items.push(
        {
          label: `Search with Google “${preview}”`,
          click: () =>
            shell.openExternal(`https://www.google.com/search?q=${encodeURIComponent(selection)}`),
        },
        { type: 'separator' },
      )
    }
    if (editable) {
      items.push(
        { role: 'cut', enabled: params.editFlags.canCut },
        { role: 'copy', enabled: params.editFlags.canCopy },
        { role: 'paste', enabled: params.editFlags.canPaste },
        { role: 'selectAll', enabled: params.editFlags.canSelectAll },
      )
    } else if (selection !== '') {
      items.push({ role: 'copy', enabled: params.editFlags.canCopy })
    }
    Menu.buildFromTemplate(items).popup({
      window: BrowserWindow.fromWebContents(contents) ?? undefined,
    })
  })
}

