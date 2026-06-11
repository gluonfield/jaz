function writeClipboardFallback(text: string) {
  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.readOnly = true
  textarea.style.position = 'fixed'
  textarea.style.opacity = '0'
  document.body.append(textarea)
  textarea.select()
  const copied = document.execCommand('copy')
  textarea.remove()
  return copied
}

export async function writeClipboard(text: string) {
  if (!navigator.clipboard?.writeText) return writeClipboardFallback(text)
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    return writeClipboardFallback(text)
  }
}
