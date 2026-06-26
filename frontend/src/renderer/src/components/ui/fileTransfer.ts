const CLIPBOARD_FILE_EXTENSIONS: Record<string, string> = {
  'image/png': 'png',
  'image/jpeg': 'jpg',
  'image/gif': 'gif',
  'image/webp': 'webp',
  'image/bmp': 'bmp',
  'image/tiff': 'tiff',
  'application/pdf': 'pdf',
}

let pastedFileSequence = 0

export function dataTransferHasFiles(data: DataTransfer | null): boolean {
  return Array.from(data?.types ?? []).includes('Files')
}

export function clipboardFiles(data: DataTransfer): File[] {
  const itemFiles = Array.from(data.items)
    .filter((item) => item.kind === 'file')
    .flatMap((item) => {
      const file = item.getAsFile()
      return file ? [normalizeClipboardFile(file, item.type)] : []
    })
  if (itemFiles.length > 0) return itemFiles
  return Array.from(data.files).map((file) => normalizeClipboardFile(file, file.type))
}

function normalizeClipboardFile(file: File, itemType: string): File {
  const type = file.type || itemType
  const name = file.name || fallbackClipboardFileName(type)
  if (name === file.name && type === file.type) return file
  return new File([file], name, { type, lastModified: file.lastModified })
}

function fallbackClipboardFileName(type: string): string {
  const id = ++pastedFileSequence
  const kind = type.startsWith('image/') ? 'image' : 'file'
  const extension = CLIPBOARD_FILE_EXTENSIONS[type]
  const suffix = extension ? `.${extension}` : ''
  return `pasted-${kind}-${id}${suffix}`
}

// Turn a captured image (data URL or `data:` base64) into an upload-ready File.
export async function dataURLToFile(dataURL: string, name: string): Promise<File> {
  const blob = await (await fetch(dataURL)).blob()
  return new File([blob], name, { type: blob.type || 'image/png' })
}
