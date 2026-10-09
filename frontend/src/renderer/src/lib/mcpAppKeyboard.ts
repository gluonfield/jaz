// Sandboxed app key events stay in the iframe; forward host navigation shortcuts.
export const mcpAppKeyboard = `<script>
window.addEventListener('keydown', (event) => {
  if (event.defaultPrevented || event.altKey || event.ctrlKey || event.shiftKey || event.repeat) return
  const key = event.code === 'BracketLeft' ? '[' : event.code === 'BracketRight' ? ']' : event.key
  const browserKey = key === 'BrowserBack' || key === 'BrowserForward'
  if (!browserKey && (!event.metaKey || !['[', ']'].includes(key) && !/^[1-9]$/.test(key))) return
  if (document.querySelector('[role="dialog"][aria-modal="true"], dialog[open]')) return
  event.preventDefault()
  window.parent.postMessage({ type: 'jaz:navigation-shortcut', key }, '*')
})
</script>`
