import { clipboardFiles } from '@/components/ui/fileTransfer'

const LARGE_PASTE_CHAR_THRESHOLD = 1000
let pastedTextSequence = 0

export function composerPasteFiles(data: DataTransfer): File[] {
  const files = clipboardFiles(data)
  if (files.length > 0) {
    return files
  }

  const text = data.getData('text/plain')
  let characters = 0
  for (const _character of text) {
    characters += 1
    if (characters > LARGE_PASTE_CHAR_THRESHOLD) {
      pastedTextSequence += 1
      return [new File([text], `pasted-text-${pastedTextSequence}.txt`, { type: 'text/plain' })]
    }
  }
  return []
}
