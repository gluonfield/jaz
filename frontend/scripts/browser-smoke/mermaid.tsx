import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { MessageMarkdown } from '@/components/session/MessageMarkdown'
import { skillsQuery } from '@/lib/api/skills'
import { getThemePref, setThemePref } from '@/lib/theme'

export async function exerciseMermaid() {
  const host = document.createElement('div')
  host.style.cssText = 'position:fixed;inset:0;width:390px;overflow:auto;background:var(--color-bg);z-index:9999'
  document.body.append(host)
  const root = createRoot(host)
  const client = new QueryClient()
  client.setQueryData(skillsQuery().queryKey, [])
  const theme = getThemePref()
  const clipboard = Object.getOwnPropertyDescriptor(navigator, 'clipboard')
  let copied = ''
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: {
      writeText: async (text: string) => {
        copied = text
      },
    },
  })
  const until = async (check: () => boolean) => {
    const deadline = performance.now() + 10000
    while (!check()) {
      if (performance.now() > deadline) {
        throw new Error('Mermaid check timed out: ' + host.textContent)
      }
      await new Promise((resolve) => setTimeout(resolve, 25))
    }
  }
  const render = (text: string) => root.render(
    <StrictMode>
      <QueryClientProvider client={client}>
        <MessageMarkdown text={text} />
      </QueryClientProvider>
    </StrictMode>,
  )
  const fence = (text: string, language = 'mermaid') => '```' + language + '\n' + text + '\n```'
  const images = () => [...host.querySelectorAll<HTMLImageElement>('img[alt="Mermaid diagram"]')]
  const svgDocument = (image: HTMLImageElement) => new DOMParser().parseFromString(decodeURIComponent(image.src.split(',')[1]), 'image/svg+xml')
  const flow = 'flowchart LR\nA["Human goal<br/>language + images"] --> B["Large multimodal planner"]\nB --> C["Robot and environment"]\nC --> A'
  try {
    setThemePref('dark')
    render([fence(flow), fence('sequenceDiagram\nAlice->>Bob: Hello', 'Mermaid'), fence('const answer = 42', 'js')].join('\n\n'))
    await until(() => images().length === 2 && images().every((image) => image.naturalWidth > 0))
    if (!host.querySelector('pre')?.textContent?.includes('const answer = 42')) {
      throw new Error('Ordinary code stopped rendering')
    }
    const diagrams = images().map(svgDocument)
    const labelRows = [...diagrams[0].querySelectorAll('text > tspan')].map((row) => row.textContent).join(' ')
    if (!labelRows.includes('language + images') || !diagrams[1].documentElement.textContent?.includes('Hello')) {
      throw new Error('Flowchart or sequence labels were lost')
    }
    const viewport = images()[0].parentElement!
    if (host.scrollWidth > host.clientWidth || viewport.scrollWidth <= viewport.clientWidth || getComputedStyle(viewport).overflowX !== 'auto') {
      throw new Error('Wide Mermaid diagrams must scroll inside the message')
    }
    host.querySelector<HTMLButtonElement>('[aria-label="Copy code"]')!.click()
    await until(() => copied === flow)
    const darkStyle = diagrams[0].querySelector('style')!.textContent!.replace(/jaz-mermaid-\d+/g, '')
    setThemePref('light')
    await until(() => {
      const diagram = images()[0]
      const style = diagram && svgDocument(diagram).querySelector('style')?.textContent?.replace(/jaz-mermaid-\d+/g, '')
      return Boolean(diagram?.naturalWidth && style && style !== darkStyle)
    })

    render(fence(String.raw`sequenceDiagram
Alice->>Bob: $$E = mc^2$$
Bob-->>Alice: $$\frac{1}{3}$$`))
    await until(() => images().some(image => {
      const svg = svgDocument(image)
      return image.naturalWidth > 0 && Boolean(svg.querySelector('math msup') && svg.querySelector('math mfrac')) && !svg.documentElement.textContent?.includes('$$')
    }))

    render(fence('flowchart LR\nA["unfinished'))
    await until(() => host.querySelector('pre')?.textContent?.includes('unfinished') === true)
    await new Promise((resolve) => setTimeout(resolve, 300))
    if (images().length || document.querySelector('[id^="djaz-mermaid-"]')) {
      throw new Error('Invalid Mermaid left a stale diagram or error SVG')
    }
    render(fence('flowchart LR\nA --> B'))
    await new Promise((resolve) => setTimeout(resolve, 170))
    render(fence('flowchart LR\nA --> C[Latest streamed node]'))
    await until(() => images().some((image) => image.naturalWidth > 0 && svgDocument(image).documentElement.textContent?.replace(/\s/g, '').includes('Lateststreamednode')))

    render(fence('%%{init: {"securityLevel": "loose"}}%%\nflowchart LR\nA[Safe] --> B[Label]\nclick A "javascript:alert(1)"'))
    await until(() => images().some((image) => image.naturalWidth > 0 && svgDocument(image).documentElement.textContent?.includes('Safe')))
    if (host.querySelector('svg a, svg script, svg [onclick]')) {
      throw new Error('A diagram enabled active content')
    }
    render(fence(flow))
    await new Promise((resolve) => setTimeout(resolve, 170))
  } finally {
    root.unmount()
    client.clear()
    host.remove()
    setThemePref(theme)
    if (clipboard) {
      Object.defineProperty(navigator, 'clipboard', clipboard)
    } else {
      Reflect.deleteProperty(navigator, 'clipboard')
    }
  }
  await new Promise((resolve) => setTimeout(resolve, 300))
  if (document.querySelector('[id^="djaz-mermaid-"]')) {
    throw new Error('Unmounted Mermaid left a rendering container')
  }
}
