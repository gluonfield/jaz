import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createRoot } from 'react-dom/client'
import { ThreadCanvas } from '@/components/canvas/ThreadCanvas'
import { applyPatch } from '@/components/canvas/board'
import { useCanvasContexts } from '@/components/canvas/useCanvasContexts'
import type { CanvasItem, CanvasView } from '@/lib/api/canvas'
import { keys } from '@/lib/query/keys'

export async function exerciseCanvasFormat(): Promise<void> {
  const sessionId = 'canvas-format'
  let view: CanvasView = {
    version: '1', dir: '/fixture', edges: [], components: {}, files: {},
    nodes: [
      { id: 'title', type: 'text', text: 'Launch plan', size: 'title', x: 0, y: 0, width: 520 },
      { id: 'caption', type: 'text', text: 'Who does what, and when', size: 'small', ink: 'blue', x: 0, y: 80, width: 320 },
      { id: 'start', type: 'text', text: 'Start', shape: 'pill', color: 'green', x: 0, y: 160, width: 140, height: 56 },
      { id: 'ship', type: 'text', text: 'Ship?', shape: 'diamond', hollow: true, x: 200, y: 130, width: 160, height: 120 },
      { id: 'store', type: 'text', text: 'Orders', shape: 'database', color: 'blue', x: 420, y: 130, width: 130, height: 120 },
      { id: 'cloud', type: 'text', text: 'Hosting', shape: 'cloud', color: '#fde2e4', x: 610, y: 130, width: 170, height: 120 },
      { id: 'note', type: 'text', text: '**Risk**\n- supplier slips\n- *no* spare tool', color: 'yellow', textAlign: 'left', x: 0, y: 300, width: 220, height: 160 },
      { id: 'grid', type: 'table', rows: [['Step', 'Owner', 'Due'], ['Draft', 'Ana', '**Mon**'], ['Review', 'Ben', 'Wed']], x: 280, y: 300, width: 420 },
    ],
  }
  const snapshot = () => ({ version: view.version, dir: view.dir, pages: [{ id: 'page-1', name: 'Page 1', nodes: view.nodes, edges: view.edges, files: view.files, components: view.components }] })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  client.setQueryData(keys.sessionCanvas(sessionId), snapshot())
  const originalFetch = window.fetch
  window.fetch = async (input, init) => {
    if (new URL(String(input)).pathname !== `/v1/sessions/${sessionId}/canvas`) {
      return originalFetch(input, init)
    }
    if (init?.method === 'PATCH') {
      view = { ...applyPatch(view, JSON.parse(String(init.body))), version: String(Number(view.version) + 1) }
      return Response.json({ version: view.version })
    }
    return Response.json(snapshot())
  }
  function Fixture() {
    const { bindSelection, onSelect, onPage } = useCanvasContexts({ contexts: [], removeContext: () => {}, replaceContexts: () => {} })
    return <ThreadCanvas sessionId={sessionId} inset={0} working={false} bindSelection={bindSelection} onSelect={onSelect} onPage={onPage} onPrompt={() => {}} />
  }
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;z-index:1;background:var(--color-bg)'
  document.body.append(element)
  const root = createRoot(element)
  const until = async (check: () => boolean, message: string) => {
    const deadline = Date.now() + 3000
    while (!check()) {
      if (Date.now() > deadline) {
        await window.smoke.capture('canvas-format-failure')
        throw new Error('Canvas format: ' + message)
      }
      await new Promise((resolve) => setTimeout(resolve, 20))
    }
  }
  const node = (id: string) => element.querySelector<HTMLElement>(`.react-flow__node[data-id="${id}"]`)!
  const saved = (id: string) => view.nodes.find((item) => item.id === id)!
  const press = async (target: Element | null | undefined, name: string) => {
    if (!target) {
      throw new Error('Canvas format: no ' + name)
    }
    const r = target.getBoundingClientRect()
    const x = Math.round(r.x + r.width / 2)
    const y = Math.round(r.y + r.height / 2)
    await window.smoke.pointer('mouseMove', x, y)
    await window.smoke.pointer('mouseDown', x, y)
    await window.smoke.pointer('mouseUp', x, y)
  }
  const select = async (id: string) => {
    await press(node(id), id)
    await until(() => node(id).classList.contains('selected') && Boolean(element.querySelector('.canvas-toolbar')), `${id} was selected without its toolbar`)
  }
  const control = (label: string) => element.querySelector(`.canvas-toolbar [aria-label="${label}"]`)
  const pick = async (menu: string, option: string) => {
    await press(control(menu), menu)
    await until(() => Boolean(element.querySelector(`.canvas-toolbar-panel[aria-label="${menu}"]`)), `${menu} did not open`)
    const options = [...element.querySelectorAll(`.canvas-toolbar-panel[aria-label="${menu}"] button`)]
    await press(options.find((button) => button.getAttribute('aria-label') === option || button.textContent === option), option)
  }
  const fontSize = (id: string, selector: string) => getComputedStyle(node(id).querySelector(selector)!).fontSize
  const place = async (tool: string, [left, right, top, bottom]: number[], check: (item: CanvasItem) => boolean) => {
    const pane = element.getBoundingClientRect()
    const free = (x: number, y: number) => Boolean(document.elementFromPoint(x, y)?.classList.contains('react-flow__pane'))
    const spot = Array.from({ length: 400 }, (_, i) => ({ x: pane.x + 60 + (i % 20) * 40, y: pane.y + 100 + Math.floor(i / 20) * 30 }))
      .find(({ x, y }) => [left, 0, right].every((dx) => [top, 0, bottom].every((dy) => free(x + dx, y + dy))))
    if (!spot) {
      throw new Error('Canvas format: no free ground for a new ' + tool)
    }
    const known = new Set(view.nodes.map((item) => item.id))
    await press(element.querySelector(`.canvas-dock [aria-label="${tool}"]`), tool + ' tool')
    await window.smoke.pointer('mouseMove', spot.x, spot.y)
    await window.smoke.pointer('mouseDown', spot.x, spot.y)
    await window.smoke.pointer('mouseUp', spot.x, spot.y)
    await until(() => view.nodes.some((item) => !known.has(item.id) && check(item)), `The ${tool} tool did not add its item`)
    return view.nodes.find((item) => !known.has(item.id))!.id
  }
  try {
    root.render(<QueryClientProvider client={client}><Fixture /></QueryClientProvider>)
    await until(() => Boolean(node('grid')?.querySelector('td p')), 'The table did not render its cells')
    await until(() => fontSize('title', 'p') === '48px' && fontSize('caption', 'p') === '14px', 'Text sizes were not applied to the words')
    if (getComputedStyle(node('caption').querySelector('p')!).color === getComputedStyle(node('title').querySelector('p')!).color) {
      throw new Error('Canvas format: coloured text kept the default ink')
    }
    for (const id of ['start', 'ship', 'store', 'cloud']) {
      const box = node(id).querySelector<HTMLElement>('.canvas-block-body')!.getBoundingClientRect()
      const words = node(id).querySelector('p')!.getBoundingClientRect()
      if (words.left < box.left - 1 || words.right > box.right + 1 || words.top < box.top - 1 || words.bottom > box.bottom + 1) {
        throw new Error(`Canvas format: the words of ${id} leave its outline`)
      }
    }
    if (!node('ship').querySelector('svg.canvas-shape path') || getComputedStyle(node('ship').querySelector('svg.canvas-shape')!).fill !== 'none') {
      throw new Error('Canvas format: an outline-only diamond was filled or not drawn')
    }
    if (getComputedStyle(node('start').querySelector('.canvas-block')!).backgroundColor === getComputedStyle(node('note').querySelector('.canvas-sticky')!).backgroundColor) {
      throw new Error('Canvas format: named fills share one colour')
    }
    if (node('grid').querySelectorAll('tr').length !== 3 || node('grid').querySelectorAll('th').length !== 3 || !node('grid').querySelector('td strong')) {
      throw new Error('Canvas format: the table lost its rows, heading or cell formatting')
    }
    const tableHeight = node('grid').querySelector('table')!.getBoundingClientRect().height / Number(getComputedStyle(element.querySelector('.react-flow__viewport')!).transform.split(',')[3])
    if (Math.abs(node('grid').offsetHeight - tableHeight) > 2) {
      throw new Error(`Canvas format: the table item is ${node('grid').offsetHeight} tall for ${tableHeight} of rows`)
    }
    await new Promise((resolve) => setTimeout(resolve, 500))
    await window.smoke.capture('canvas-format')

    await select('title')
    await pick('Text size', 'Large')
    await until(() => saved('title').size === 'large' && fontSize('title', 'p') === '28px', 'The size menu did not resize the words')
    await press(control('Bold'), 'Bold')
    await until(() => saved('title').text === '**Launch plan**', 'Bold did not mark the text')
    await press(control('Bold'), 'Bold')
    await until(() => saved('title').text === 'Launch plan', 'Bold did not clear its mark')
    await press(control('Bulleted list'), 'Bulleted list')
    await until(() => saved('title').text === '- Launch plan' && Boolean(node('title').querySelector('li')), 'The list control did not make a list')
    await press(control('Align left'), 'Align')
    await until(() => saved('title').textAlign === 'center', 'Align did not save')
    await pick('Text colour', 'Purple')
    await until(() => saved('title').ink === 'purple', 'Text colour did not save')
    await pick('Shape', 'Hexagon')
    await until(() => saved('title').shape === 'hexagon' && typeof saved('title').height === 'number' && Boolean(node('title').querySelector('svg.canvas-shape')), 'Text did not become a shape with a height')
    await pick('Fill', 'Orange')
    await until(() => saved('title').color === 'orange', 'Fill did not save')
    await pick('Shape', 'Text')
    await until(() => saved('title').shape === undefined && saved('title').color === undefined && saved('title').height === undefined && !node('title').querySelector('.canvas-block'), 'A shape did not return to plain text')

    await select('note')
    node('note').querySelector('.canvas-sticky')!.dispatchEvent(new MouseEvent('dblclick', { bubbles: true }))
    await until(() => Boolean(document.activeElement?.classList.contains('canvas-text-editor')), 'The note did not open for typing')
    const editor = document.activeElement as HTMLTextAreaElement
    editor.setSelectionRange(11, 19)
    await press(control('Italic'), 'Italic')
    await until(() => editor.value.includes('- *supplier* slips') && document.activeElement === editor && editor.selectionStart === 12 && editor.selectionEnd === 20, 'Italic did not wrap the selected words and keep them selected')
    await window.smoke.key('Escape')
    await until(() => String(saved('note').text).includes('- *supplier* slips'), 'Formatting made while typing was not saved')

    await select('grid')
    const cell = (row: number, column: number) => node('grid').querySelectorAll('tr')[row].children[column]
    cell(1, 1).dispatchEvent(new MouseEvent('dblclick', { bubbles: true }))
    await until(() => Boolean(cell(1, 1).querySelector('textarea') === document.activeElement), 'A table cell did not open for typing')
    await window.smoke.key('Z')
    await window.smoke.key('Tab')
    await until(() => (saved('grid').rows as string[][])[1][1].toLowerCase() === 'z' && cell(1, 2).querySelector('textarea') === document.activeElement, 'Tab did not keep the cell and move to the next')
    await window.smoke.key('Escape')
    await until(() => !node('grid').querySelector('textarea') && (saved('grid').rows as string[][])[1][2] === '**Mon**', 'Escape changed the cell it left')
    await press(node('grid').querySelector('.canvas-table-add.row'), 'Add row')
    await until(() => (saved('grid').rows as string[][]).length === 4 && node('grid').querySelectorAll('tr').length === 4, 'A row was not added')
    await press(node('grid').querySelector('.canvas-table-add.column'), 'Add column')
    await until(() => (saved('grid').rows as string[][]).every((row) => row.length === 4), 'A column was not added')
    await press(cell(2, 0), 'cell')
    await press(control('Remove row'), 'Remove row')
    await until(() => (saved('grid').rows as string[][]).length === 3 && (saved('grid').rows as string[][])[2][0] === '', 'The row of the chosen cell was not removed')

    for (let step = 0; step < 2; step++) {
      element.querySelector<HTMLButtonElement>('[aria-label="Zoom out"]')!.click()
      await new Promise((resolve) => setTimeout(resolve, 250))
    }
    const block = await place('Shape', [-110, 110, -70, 70], (item) => item.shape === 'rounded' && typeof item.height === 'number')
    await until(() => Boolean(document.activeElement?.classList.contains('canvas-text-editor')) && node(block).classList.contains('selected') && Boolean(control('Bold')), 'A new shape did not open for typing with its controls')
    await window.smoke.key('k')
    await window.smoke.key('Escape')
    await until(() => saved(block).text === 'k', 'Words typed in a new shape were not saved')
    const table = await place('Table', [0, 430, 0, 140], (item) => item.type === 'table' && item.height === undefined)
    await until(() => node(table)?.querySelectorAll('th, td').length === 9, 'A new table did not show its nine cells')
    await new Promise((resolve) => setTimeout(resolve, 500))
    await window.smoke.capture('canvas-format-edited')
  } finally {
    root.unmount()
    client.clear()
    element.remove()
    window.fetch = originalFetch
  }
}
