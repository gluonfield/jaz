export function accessibilityFixture(origin: string): string {
  return `<!doctype html><html><head><title>AX fixture</title></head><body>
<h1>Accessibility fixture</h1>
<label for="answer">Computed field label</label><input id="answer">
<input type="password" aria-label="Password" value="secret-must-not-appear">
<span aria-hidden="true">Hidden exclusion sentinel</span>
<button aria-label="Update status" id="update">Update</button>
<output role="status">Ready for AX</output>
<div id="closed-shadow"></div>
<iframe title="Same site" style="border:18px solid silver;padding:12px;transform:scale(.9)" src="${origin}/accessibility-frame?same"></iframe>
<iframe title="Cross site" style="border:18px solid silver;padding:12px;transform:scale(.9)" src="${origin.replace('127.0.0.1', 'localhost')}/accessibility-frame?cross"></iframe>
<iframe aria-hidden="true" title="Hidden frame" srcdoc="<button>Hidden frame action</button>"></iframe>
<div style="width:140px"><span role="button" tabindex="0" id="fragment" aria-label="Wrapped action">A long clickable sentence that wraps onto several lines</span></div>
<button id="offset" style="display:block;width:350px;text-align:left">Offset label</button>
<button id="smooth" style="display:block;margin-top:1200px">Smooth scrolling action</button>
<script>
document.getElementById('update').onclick = event => {
  document.querySelector('output').textContent = 'Verified AX action'
  window.trustedAX = event.isTrusted
}
const shadow = document.getElementById('closed-shadow').attachShadow({mode:'closed'})
const shadowButton = document.createElement('button')
shadowButton.textContent = 'Closed shadow action'
shadowButton.onclick = event => {
  window.trustedShadow = event.isTrusted
}
shadow.append(shadowButton)
document.getElementById('fragment').onclick = event => {
  window.trustedFragment = event.isTrusted
}
document.getElementById('offset').onclick = event => {
  window.trustedOffset = event.isTrusted
}
document.getElementById('smooth').onclick = event => {
  window.trustedSmooth = event.isTrusted
}
window.addEventListener('message', event => {
  if (event.data?.kind === 'frame-action') {
    window[event.data.name] = event.data.trusted
  }
})
</script></body></html>`
}

export function accessibilityFrame(url: URL): string {
  const cross = url.searchParams.has('cross')
  const nested = url.searchParams.has('nested')
  const name = (nested ? 'Nested ' : '') + (cross ? 'Cross-site action' : 'Same-site action')
  return `<!doctype html><html><head><title>${name}</title></head><body>
<button id="action">${name}</button>
<label for="value">${name} field</label><input id="value">
${cross && !nested ? `
<iframe title="Nested same site" width="130" height="50" src="${url.origin}/accessibility-frame?nested"></iframe>
<iframe title="Nested cross site" width="130" height="50" src="${url.origin.replace('localhost', '127.0.0.1')}/accessibility-frame?nested&cross"></iframe>` : ''}
<script>
document.getElementById('action').onclick = event => {
  top.postMessage({kind:'frame-action',name:${JSON.stringify(name)},trusted:event.isTrusted},'*')
}
</script>
</body></html>`
}
