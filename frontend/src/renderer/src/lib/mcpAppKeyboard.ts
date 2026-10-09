// Sandboxed app key events stay in the iframe; forward host navigation shortcuts.
export const mcpAppKeyboard = `<script>
window.addEventListener('keydown', (event) => {
  if (event.defaultPrevented || event.altKey || event.ctrlKey || event.shiftKey || event.repeat) return
  const browserKey = event.key === 'BrowserBack' || event.key === 'BrowserForward'
  if (!browserKey && (!event.metaKey || !['[', ']'].includes(event.key) && !/^[1-9]$/.test(event.key))) return
  if (document.querySelector('[role="dialog"][aria-modal="true"], dialog[open]')) return
  event.preventDefault()
  window.parent.postMessage({ type: 'jaz:navigation-shortcut', key: event.key }, '*')
})
</script>`
