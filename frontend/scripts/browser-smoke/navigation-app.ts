import { App, PostMessageTransport } from '@modelcontextprotocol/ext-apps'
import { createMemoryHistory } from '@tanstack/react-router'

const app = new App({ name: 'Navigation peer', version: '1' }, {
  availableDisplayModes: ['fullscreen'],
  experimental: { 'jaz/navigation': {} },
})
const history = createMemoryHistory({ initialEntries: ['/people'] })
const instance = Math.random()
const report = (replace: boolean, delta?: number) => app.notification({
  method: 'jaz/notifications/navigation',
  params: { path: history.location.href, replace, ...(delta && { delta }) },
})
const paint = () => {
  document.body.innerHTML = '<button id="person">Jim</button><button id="other">Jane</button><button id="filter">Filter</button><button id="replace">Replace</button><button id="back">Back</button>'
  document.getElementById('person')!.onclick = () => history.push('/r/jim')
  document.getElementById('other')!.onclick = () => history.push('/r/jane')
  document.getElementById('filter')!.onclick = () => history.push('/people?tags=Advisor')
  document.getElementById('replace')!.onclick = () => history.replace('/people?tags=Customer')
  document.getElementById('back')!.onclick = () => history.back()
  window.parent.postMessage({ type: 'navigation-peer', path: history.location.href, instance }, '*')
}
history.subscribe(({ location, action }) => {
  paint()
  const delta = action.type === 'BACK' ? -1 : action.type === 'FORWARD' ? 1 : action.type === 'GO' ? action.index : undefined
  const path = (app.getHostContext()?.['jaz/navigation'] as { path?: string } | undefined)?.path
  if (delta || location.href !== path) void report(action.type !== 'PUSH', delta)
})
app.onhostcontextchanged = (context) => {
  const path = (context['jaz/navigation'] as { path?: string } | undefined)?.path
  if (path && path !== history.location.href) history.replace(path)
}
app.ontoolinput = ({ arguments: input }) => {
  if (typeof input?.path === 'string') history.push(input.path)
  else history.replace('/people')
  void report(true)
}
window.addEventListener('message', (event) => {
  if (event.source !== window.parent || event.data?.type !== 'navigation-peer-action') return
  if (event.data.key) window.dispatchEvent(new KeyboardEvent('keydown', { key: event.data.key, code: event.data.code, metaKey: true, cancelable: true }))
  else document.getElementById(event.data.button)?.click()
})
await app.connect(new PostMessageTransport(window.parent, window.parent))
app.onhostcontextchanged(app.getHostContext()!)
paint()
await report(true)
