import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useLayoutEffect, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { FileReaderPanel } from '@/components/session/FileReaderPanel'
import { FileReaderLinkProvider, MessageMarkdown } from '@/components/session/MessageMarkdown'
import { MarkdownArtifactContext } from '@/components/session/MarkdownArtifact'
import { keys } from '@/lib/query/keys'
import { getThemePref, setThemePref } from '@/lib/theme'
import type { FileReference } from '@shared/fileReader'

export async function exerciseMarkdownArtifacts() {
  const path = '/workspace/report.md'
  const code = `<!doctype html><html><head><style>body{margin:0;color:var(--color-ink);font-family:var(--font-sans)}#panel{height:160px;padding:12px}button{margin:4px;padding:12px;color:inherit;background:var(--color-surface);border:1px solid var(--color-border)}</style></head><body>
<div id="panel"><button id="advance">Advance</button><output>0</output><button id="ask">Ask Jaz</button><button id="fail">Show error</button></div>
<script>
let count = 0;
const report = () => requestAnimationFrame(() => {
  let isolated = false;
  try {
    parent.document.body;
  } catch {
    isolated = true;
  }
  const buttons = Object.fromEntries([...document.querySelectorAll('button')].map(button => {
    const rect = button.getBoundingClientRect();
    return [button.id, { x: rect.x + rect.width / 2, y: rect.y + rect.height / 2 }];
  }));
  parent.postMessage({ type: 'fixture:markdown-artifact', count, isolated, scheme: getComputedStyle(document.documentElement).colorScheme, csp: document.head.querySelector('meta[http-equiv]')?.httpEquiv.toLowerCase() === ['content', 'security', 'policy'].join('-'), buttons }, '*');
});
document.getElementById('advance').onclick = () => {
  document.querySelector('output').textContent = String(++count);
  document.getElementById('panel').style.height = '360px';
  report();
};
document.getElementById('ask').onclick = () => sendPrompt('Explain this comparison');
document.getElementById('fail').onclick = () => {
  throw new Error('Expected artifact error');
};
window.addEventListener('load', report);
window.addEventListener('resize', report);
report();
</script></body></html>`
  const content = '# Artifact report\n\n~~~jaz-artifact Route counter\n' + code + '\n~~~\n\n## Notes\n\nAn ordinary document with an interactive figure.'
  const client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity, retry: false } } })
  client.setQueryData(keys.sessionFile('artifacts', path), { path, content, size: content.length })
  client.setQueryData(keys.health, { capabilities: { session_file_read: true } })
  client.setQueryData(keys.skills(), [])
  const host = document.createElement('div')
  host.style.cssText = 'position:fixed;inset:0;z-index:10;background:var(--color-bg);padding:16px;overflow:auto'
  document.body.append(host)
  const root = createRoot(host)
  const prompts: string[] = []
  let setActive!: (active: boolean) => void
  function View() {
    const [file, setFile] = useState<FileReference | null>(null)
    const [active, updateActive] = useState(true)
    useLayoutEffect(() => { setActive = updateActive })
    return <QueryClientProvider client={client}>
      <MarkdownArtifactContext value={{ mode: 'source', onSendPrompt: (text) => prompts.push(text) }}>
        <FileReaderLinkProvider sessionId="artifacts" onOpen={setFile}>
          <MessageMarkdown text={path} />
          {file ? <div hidden={!active} data-artifact-reader className="h-[65vh] border border-border"><FileReaderPanel sessionId="artifacts" fileRef={file} visible={active} onOpenFile={setFile} onClose={() => setFile(null)} /></div> : null}
        </FileReaderLinkProvider>
      </MarkdownArtifactContext>
    </QueryClientProvider>
  }
  type Probe = { count: number; isolated: boolean; scheme: string; csp: boolean; buttons: Record<string, { x: number; y: number }> }
  let probe: Probe | undefined
  const frame = () => host.querySelector<HTMLIFrameElement>('[data-artifact-reader] iframe')
  const receive = (event: MessageEvent) => {
    if (event.source === frame()?.contentWindow && event.data?.type === 'fixture:markdown-artifact') probe = event.data
  }
  window.addEventListener('message', receive)
  const until = async (check: () => boolean) => {
    const deadline = Date.now() + 5000
    while (!check()) {
      if (Date.now() > deadline) throw new Error('Markdown artifact check: ' + check.toString())
      await new Promise((resolve) => setTimeout(resolve, 20))
    }
  }
  const point = async (x: number, y: number) => {
    await window.smoke.pointer('mouseMove', x, y)
    await window.smoke.pointer('mouseDown', x, y)
    await window.smoke.pointer('mouseUp', x, y)
  }
  const button = async (id: string) => {
    const current = frame()!
    current.scrollIntoView({ block: 'start' })
    await new Promise(requestAnimationFrame)
    const rect = current.getBoundingClientRect()
    await point(Math.round(rect.x + probe!.buttons[id].x), Math.round(rect.y + probe!.buttons[id].y))
  }
  const theme = getThemePref()
  try {
    await window.smoke.resize(1000, 844)
    setThemePref('dark')
    root.render(<View />)
    await until(() => Boolean(host.querySelector('.markdown-document-card button')))
    if (host.querySelector('iframe') || !host.textContent?.includes('Route counter · Interactive artifact')) throw new Error('Compact preview executed its artifact')
    const open = host.querySelector('.markdown-document-card button')!.getBoundingClientRect()
    await point(Math.round(open.x + open.width / 2), Math.round(open.y + open.height / 2))
    await until(() => probe?.scheme === 'dark')
    if (!probe?.isolated || !probe.csp || frame()?.getAttribute('sandbox') !== 'allow-scripts allow-forms' || frame()?.getAttribute('referrerpolicy') !== 'no-referrer') throw new Error('Embedded artifact lost the existing sandbox boundary')
    window.dispatchEvent(new MessageEvent('message', { source: window, data: { type: 'jaz:artifact-send-prompt', text: 'Forged prompt' } }))
    if (prompts.length) throw new Error('Artifact accepted a message from outside its frame')
    await button('advance')
    await until(() => probe?.count === 1 && parseInt(frame()?.style.height ?? '') >= 360)
    await button('ask')
    await until(() => prompts.length === 1)
    if (prompts[0] !== 'Explain this comparison') throw new Error('Artifact prompt bypassed the provided host callback')
    probe = undefined
    setThemePref('light')
    await until(() => probe?.scheme === 'light')
    await window.smoke.capture('markdown-artifact-reader')
    setActive(false)
    await until(() => !frame())
    probe = undefined
    setActive(true)
    await until(() => probe?.scheme === 'light')
    await button('fail')
    await until(() => host.textContent?.includes('Expected artifact error') === true)
  } finally {
    root.unmount()
    host.remove()
    client.clear()
    window.removeEventListener('message', receive)
    setThemePref(theme)
  }
}
