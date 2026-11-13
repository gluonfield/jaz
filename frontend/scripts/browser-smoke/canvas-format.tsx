import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createRoot } from 'react-dom/client'
import { ThreadCanvas } from '@/components/canvas/ThreadCanvas'
import { applyPatch } from '@/components/canvas/board'
import { useCanvasContexts } from '@/components/canvas/useCanvasContexts'
import type { CanvasItem, CanvasLayout, CanvasView } from '@/lib/api/canvas'
import { keys } from '@/lib/query/keys'

export async function exerciseCanvasFormat(): Promise<void> {
  const sessionId = 'canvas-format'
  let view: CanvasView = {
    version: '1', dir: '/fixture', components: {}, files: { counter: { path: 'counter.html', version: '1' } },
    edges: [{ id: 'flow', fromNode: 'start', toNode: 'ship', label: 'ready', route: 'elbow', dash: 'dashed', color: 'blue', toEnd: 'circle' }],
    nodes: [
      { id: 'title', type: 'text', text: 'Launch plan', size: 'title', x: 0, y: 0, width: 520 },
      { id: 'caption', type: 'text', text: 'Who does what, and when', size: 'small', ink: 'blue', x: 0, y: 80, width: 320 },
      { id: 'start', type: 'text', text: 'Start', shape: 'pill', color: 'green', x: 0, y: 160, width: 140, height: 56 },
      { id: 'ship', type: 'text', text: 'Ship?', shape: 'diamond', hollow: true, x: 200, y: 130, width: 160, height: 120 },
      { id: 'store', type: 'text', text: 'Orders', shape: 'database', color: 'blue', x: 420, y: 130, width: 130, height: 120 },
      { id: 'cloud', type: 'text', text: 'Hosting', shape: 'cloud', color: '#fde2e4', x: 610, y: 130, width: 170, height: 120 },
      { id: 'note', type: 'text', text: '**Risk**\n- supplier slips\n- *no* spare tool', color: 'yellow', textAlign: 'left', x: 0, y: 300, width: 220, height: 160 },
      { id: 'grid', type: 'table', rows: [['Step', 'Owner', 'Due'], ['Draft', 'Ana', '**Mon**'], ['Review', 'Ben', 'Wed']], x: 280, y: 300, width: 420 },
      { id: 'counter', type: 'file', file: 'counter.html', data: { count: 2 }, x: 0, y: 520, width: 220, height: 120 },
      { id: 'timer', type: 'timer', seconds: 300, x: 280, y: 520, width: 220, height: 120 },
      { id: 'placed', type: 'text', text: 'Placed under the table', x: 0, y: 0, width: 300, place: { below: 'grid', gap: 24 } },
    ],
  }
  const snapshot = () => ({ version: view.version, dir: view.dir, pages: [{ id: 'page-1', name: 'Page 1', nodes: view.nodes, edges: view.edges, files: view.files, components: view.components }] })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  client.setQueryData(keys.sessionCanvas(sessionId), snapshot())
  client.setQueryData([...keys.sessionFile(sessionId, 'counter.html'), '1'], { content: `
    <button id="add" style="display:block;width:100%;height:100px"></button>
    <script>
      const add = document.getElementById('add')
      const show = () => { add.textContent = 'Count ' + window.data.count }
      show()
      sendPrompt('loaded ' + window.data.count)
      onData((data) => { show(); sendPrompt('heard ' + data.count) })
      add.onclick = () => { saveData({ count: window.data.count + 1 }); show() }
    </script>` })
  const heard: string[] = []
  const originalFetch = window.fetch
  const uploads: string[] = []
  let layout: CanvasLayout = { page: '', items: [], issues: [] }
  window.fetch = async (input, init) => {
    if (new URL(String(input)).pathname === `/v1/sessions/${sessionId}/canvas/layout`) {
      layout = JSON.parse(String(init?.body))
      return new Response(null, { status: 204 })
    }
    if (new URL(String(input)).pathname === `/v1/sessions/${sessionId}/canvas/files`) {
      const sent = (init!.body as FormData).get('file') as File
      uploads.push(`${sent.name}:${sent.size}`)
      return Response.json({ file: `uploads/${sent.name}` })
    }
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
    return <ThreadCanvas sessionId={sessionId} inset={0} working={false} bindSelection={bindSelection} onSelect={onSelect} onPage={onPage} onPrompt={(text) => heard.push(text)} />
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
  const dockTool = (tool: string) => element.querySelector(`.canvas-dock [aria-label="${tool}"]`)
  const boardZoom = () => Number(new DOMMatrix(getComputedStyle(element.querySelector('.react-flow__viewport')!).transform).a)
  const fontSize = (id: string, selector: string) => getComputedStyle(node(id).querySelector(selector)!).fontSize
  const hover = async (target: Element | null | undefined) => {
    const r = target!.getBoundingClientRect()
    await window.smoke.pointer('mouseMove', Math.round(r.x + r.width / 2), Math.round(r.y + r.height / 2))
  }
  // A tool on the dock, or one listed over the dock tool named by `under`. A
  // tool that outlines its item under the pointer must put it down there.
  const place = async (tool: string, [left, right, top, bottom]: number[], check: (item: CanvasItem) => boolean, under?: string) => {
    const pane = element.getBoundingClientRect()
    const free = (x: number, y: number) => Boolean(document.elementFromPoint(x, y)?.classList.contains('react-flow__pane'))
    const spot = Array.from({ length: 400 }, (_, i) => ({ x: pane.x + 60 + (i % 20) * 40, y: pane.y + 100 + Math.floor(i / 20) * 30 }))
      .find(({ x, y }) => [left, 0, right].every((dx) => [top, 0, bottom].every((dy) => free(x + dx, y + dy))))
    if (!spot) {
      throw new Error('Canvas format: no free ground for a new ' + tool)
    }
    const known = new Set(view.nodes.map((item) => item.id))
    if (under) {
      await hover(dockTool(under))
      await until(() => Boolean(element.querySelector(`.canvas-dock-panel [aria-label="${tool}"]`)), `Pointing at ${under} did not list ${tool}`)
      await until(() => getComputedStyle(element.querySelector('.canvas-dock-panel')!, '::after').content === '"Shapes  R"' && getComputedStyle(element.querySelector('.canvas-dock-panel')!, '::after').opacity === '1', `Pointing at ${under} did not show its name`)
    }
    await press(element.querySelector(`${under ? '.canvas-dock-panel' : '.canvas-dock'} [aria-label="${tool}"]`), tool + ' tool')
    await until(() => !element.querySelector('.canvas-dock-panel'), `Choosing ${tool} left the list open`)
    await window.smoke.pointer('mouseMove', spot.x, spot.y)
    await new Promise((resolve) => setTimeout(resolve, 60))
    const ghost = element.querySelector('.canvas-tool-layer .canvas-ghost')
    const outline = ghost && { box: ghost.getBoundingClientRect(), path: ghost.querySelector('path')!.getAttribute('d') }
    if (!outline && tool !== 'Sticker') {
      throw new Error(`Canvas format: the ${tool} tool did not outline its item under the pointer`)
    }
    await window.smoke.pointer('mouseDown', spot.x, spot.y)
    await window.smoke.pointer('mouseUp', spot.x, spot.y)
    await until(() => view.nodes.some((item) => !known.has(item.id) && check(item)), `The ${tool} tool did not add its item`)
    const id = view.nodes.find((item) => !known.has(item.id))!.id
    await until(() => Boolean(node(id)), `The new ${tool} was not drawn`)
    const landed = node(id).getBoundingClientRect()
    const drawn = node(id).querySelector('.canvas-shape path')?.getAttribute('d')
    if (outline && ((drawn && drawn !== outline.path) || [outline.box.x - landed.x, outline.box.y - landed.y, outline.box.width - landed.width].some((gap) => Math.abs(gap) > 1.5))) {
      throw new Error(`Canvas format: the ${tool} was not outlined where it then landed: ` + JSON.stringify([outline, landed, drawn]))
    }
    return id
  }
  try {
    root.render(<QueryClientProvider client={client}><Fixture /></QueryClientProvider>)
    await until(() => Boolean(node('grid')?.querySelector('td p')), 'The table did not render its cells')
    await until(() => heard.includes('loaded 2'), 'A card did not start with its saved data')
    const under = () => 324 + node('grid').offsetHeight
    await until(() => saved('placed').place === undefined && saved('placed').x === 280 && Math.abs(Number(saved('placed').y) - under()) <= 1, 'An item placed below the table did not take its spot from the table\'s measured height')
    const seen = (id: string) => layout.items.find((item) => item.id === id)
    await until(() => layout.page === 'page-1' && seen('placed')?.y === saved('placed').y && Math.abs((seen('grid')?.height ?? 0) - node('grid').offsetHeight) <= 1 && seen('start')?.height === 56, 'The measured layout was not reported')
    if (layout.issues.length) {
      throw new Error('Canvas format: a clean page reported ' + layout.issues.join('; '))
    }
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
    const line = () => element.querySelector<SVGPathElement>('.react-flow__edge[data-id="flow"] path.canvas-edge')!
    await until(() => Boolean(line()), 'The connector was not drawn')
    if (Number(getComputedStyle(line().closest('svg')!).zIndex) <= Math.max(...['start', 'ship', 'note'].map((id) => Number(getComputedStyle(node(id)).zIndex) || 0))) {
      throw new Error('Canvas format: the connector was not drawn over the items')
    }
    if (getComputedStyle(line()).strokeDasharray === 'none' || getComputedStyle(line()).stroke !== 'rgb(47, 128, 237)' || line().getAttribute('d')!.includes('C') || !element.querySelector('.react-flow__edge[data-id="flow"] .canvas-edge-dot')) {
      throw new Error('Canvas format: the connector lost its route, dashes, colour or round end')
    }
    await window.smoke.capture('canvas-format')
    const on = line().getPointAtLength(line().getTotalLength() * 0.2)
    const frame = line().getScreenCTM()!
    await window.smoke.pointer('mouseMove', Math.round(on.x * frame.a + frame.e), Math.round(on.y * frame.d + frame.f))
    await window.smoke.pointer('mouseDown', Math.round(on.x * frame.a + frame.e), Math.round(on.y * frame.d + frame.f))
    await window.smoke.pointer('mouseUp', Math.round(on.x * frame.a + frame.e), Math.round(on.y * frame.d + frame.f))
    const connector = (label: string) => element.querySelector(`.canvas-edge-toolbar [aria-label="${label}"]`)
    await until(() => Boolean(connector('Route: elbow')), 'A selected connector did not show its controls')
    await press(connector('Route: elbow'), 'Route')
    await press(connector('Line: dashed'), 'Line')
    await press(connector('End: circle'), 'End')
    await press(connector('Start: none'), 'Start')
    await until(() => view.edges[0].route === 'straight' && view.edges[0].dash === 'dotted' && view.edges[0].toEnd === 'none' && view.edges[0].fromEnd === 'arrow', 'Connector controls did not save: ' + JSON.stringify(view.edges[0]))
    await until(() => /^M[^CQ]*$/.test(line().getAttribute('d')!) && !element.querySelector('.react-flow__edge[data-id="flow"] .canvas-edge-dot'), 'The connector was not redrawn straight without its round end')
    await press(connector('Line colour'), 'Line colour')
    await until(() => Boolean(connector('Green')), 'The connector colours did not open')
    await press(connector('Green'), 'Green')
    await until(() => view.edges[0].color === 'green' && getComputedStyle(line()).stroke !== 'rgb(47, 128, 237)', 'Connector colour did not save')
    await press(element.querySelector('.react-flow__pane'), 'board')

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
    const editor = document.activeElement as HTMLElement
    if (!editor.isContentEditable || !editor.querySelector('strong') || editor.querySelectorAll('li').length !== 2 || editor.textContent!.includes('*')) {
      throw new Error('Canvas format: typing did not start in the formatted words: ' + editor.outerHTML)
    }
    const words = [...editor.querySelectorAll('li')][0].firstChild!
    const range = document.createRange()
    range.setStart(words, 0)
    range.setEnd(words, 8)
    getSelection()!.removeAllRanges()
    getSelection()!.addRange(range)
    await press(control('Italic'), 'Italic')
    await until(() => editor.querySelector('li')!.firstElementChild?.textContent === 'supplier' && document.activeElement === editor && getSelection()!.toString() === 'supplier', 'Italic did not format the selected words and keep them selected')
    await press(control('Strikethrough'), 'Strikethrough')
    getSelection()!.selectAllChildren(editor)
    getSelection()!.collapseToEnd()
    for (const key of ['Enter', 'o', 'k']) {
      await window.smoke.key(key)
    }
    await window.smoke.key('Escape')
    await until(() => saved('note').text === '**Risk**\n\n- ~~*supplier*~~ slips\n- *no* spare tool\n- ok', 'Formatted typing was not saved as Markdown: ' + JSON.stringify(saved('note').text))
    await press(control('Numbered list'), 'Numbered list')
    await until(() => saved('note').text === '1. **Risk**\n2. ~~*supplier*~~ slips\n3. *no* spare tool\n4. ok', 'The numbered list control did not renumber the lines')
    view = { ...applyPatch(view, { nodes: [{ id: 'caption', text: '## Heading kept as source' }] }), version: String(Number(view.version) + 1) }
    await client.invalidateQueries({ queryKey: keys.sessionCanvas(sessionId) })
    await until(() => Boolean(node('caption').querySelector('h2')), 'The agent heading did not render')
    await select('caption')
    node('caption').querySelector('.canvas-text')!.dispatchEvent(new MouseEvent('dblclick', { bubbles: true }))
    await until(() => document.activeElement instanceof HTMLTextAreaElement && document.activeElement.value === '## Heading kept as source', 'Markdown beyond formatted words did not open as source')
    await window.smoke.key('Escape')

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
    await press(control('Heading row'), 'Heading row')
    await until(() => saved('grid').header === false && !node('grid').querySelector('th'), 'The heading row did not turn off')
    await press(control('Heading row'), 'Heading row')
    await until(() => saved('grid').header === undefined && Boolean(node('grid').querySelector('th')), 'The heading row did not turn back on')
    await select('placed')
    await press(control('Checklist'), 'Checklist')
    await until(() => saved('placed').text === '- [ ] Placed under the table' && Boolean(node('placed').querySelector('input[type="checkbox"]')), 'Checklist did not make the line a task')
    await press(node('placed').querySelector('input[type="checkbox"]'), 'task box')
    await until(() => saved('placed').text === '- [x] Placed under the table', 'A click on a task\'s box did not tick it')
    node('placed').querySelector('.canvas-text')!.dispatchEvent(new MouseEvent('dblclick', { bubbles: true }))
    await until(() => Boolean(node('placed').querySelector('.canvas-rich-editor li[data-checked="true"]')), 'A checklist did not open as formatted words')
    await press(control('Checklist'), 'Checklist')
    await window.smoke.key('Escape')
    await until(() => saved('placed').text === '- Placed under the table', 'Checklist did not turn a task back into a bullet while typing')

    for (let step = 0; step < 2; step++) {
      element.querySelector<HTMLButtonElement>('[aria-label="Zoom out"]')!.click()
      await new Promise((resolve) => setTimeout(resolve, 250))
    }
    const many = () => element.querySelector('.canvas-toolbar[aria-label="Selection actions"]')
    const lasso = async () => {
      const ship = node('ship').getBoundingClientRect()
      const first = node('start').getBoundingClientRect()
      const corners = [first.left - 24, Math.min(first.top, ship.top) - 16, ship.right + 16, Math.max(first.bottom, ship.bottom) + 16].map(Math.round)
      await window.smoke.pointer('mouseMove', corners[0], corners[1])
      await window.smoke.pointer('mouseDown', corners[0], corners[1])
      await window.smoke.pointer('mouseMove', Math.round((corners[0] + corners[2]) / 2), Math.round((corners[1] + corners[3]) / 2))
      await window.smoke.pointer('mouseMove', corners[2], corners[3])
      await window.smoke.pointer('mouseUp', corners[2], corners[3])
      await until(() => node('start').classList.contains('selected') && node('ship').classList.contains('selected') && Boolean(many()), 'Two selected items did not show the selection controls')
    }
    await lasso()
    await press(many()!.querySelector('[aria-label="Align top"]'), 'Align top')
    await until(() => saved('start').y === saved('ship').y, 'Align top did not line the items up')
    const loose = view.nodes.length
    await press(many()!.querySelector('[aria-label="Group"]'), 'Group')
    await until(() => view.nodes.length === loose + 1 && view.nodes.at(-1)!.type === 'group' && view.nodes.at(-1)!.x === Number(saved('start').x) - 36 && view.nodes.at(-1)!.y === Number(saved('start').y) - 36, 'Group did not put a section around the selection')
    const section = view.nodes.at(-1)!.id
    await press(element.querySelector('.react-flow__pane'), 'board')
    await until(() => !many(), 'A click on the board did not clear the selection')
    await press(node(section).querySelector('.canvas-section-label'), 'section label')
    await until(() => node(section).classList.contains('selected') && Boolean(control('Ungroup')), 'The section did not offer Ungroup')
    await press(control('Ungroup'), 'Ungroup')
    await until(() => view.nodes.length === loose && Boolean(saved('start')) && Boolean(saved('ship')), 'Ungroup did not remove the section alone')
    await lasso()
    await press(many()!.querySelector('[aria-label="Lock"]'), 'Lock')
    await until(() => saved('start').locked === true && saved('ship').locked === true, 'Lock did not save')
    await press(element.querySelector('.react-flow__pane'), 'board')
    const held = node('start').getBoundingClientRect()
    await window.smoke.pointer('mouseMove', Math.round(held.x + held.width / 2), Math.round(held.y + held.height / 2))
    await window.smoke.pointer('mouseDown', Math.round(held.x + held.width / 2), Math.round(held.y + held.height / 2))
    await window.smoke.pointer('mouseMove', Math.round(held.x + held.width / 2 + 40), Math.round(held.y + held.height / 2))
    await window.smoke.pointer('mouseUp', Math.round(held.x + held.width / 2 + 40), Math.round(held.y + held.height / 2))
    await until(() => node('start').classList.contains('selected') && Boolean(control('Unlock')) && !control('Bold'), 'A locked item did not offer only its lock')
    if (Math.abs(node('start').getBoundingClientRect().x - held.x) > 1) {
      throw new Error('Canvas format: a locked item moved')
    }
    await press(control('Unlock'), 'Unlock')
    await until(() => saved('start').locked === undefined && Boolean(control('Bold')), 'Unlock did not restore the controls')
    await pick('More', 'Bring to front')
    await until(() => Number(saved('start').z) > 0 && Number(getComputedStyle(node('start')).zIndex) > 0, 'Bring to front did not restack the item')
    const count = view.nodes.length
    await pick('More', 'Duplicate')
    await until(() => view.nodes.length === count + 1 && view.nodes.at(-1)!.shape === 'pill', 'Duplicate did not copy the item')
    await until(() => Boolean(control('More')) && node(view.nodes.at(-1)!.id)?.classList.contains('selected'), 'The copy was not selected')
    await until(() => layout.issues.some((issue) => issue.startsWith(`\`start\` overlaps \`${view.nodes.at(-1)!.id}\``)), 'A copy on top of its original was not reported as an overlap')
    await pick('More', 'Delete')
    await until(() => view.nodes.length === count, 'Delete did not remove the item')
    await until(() => !layout.issues.some((issue) => issue.includes('overlaps')), 'The overlap stayed reported after the copy was deleted')
    await select('start')
    await pick('More', 'Copy style')
    await select('cloud')
    await pick('More', 'Paste style')
    await until(() => saved('cloud').shape === 'pill' && saved('cloud').color === 'green' && saved('cloud').text === 'Hosting', 'Paste style did not give the item the copied look')

    const block = await place('Shapes', [-110, 110, -70, 70], (item) => item.shape === 'rounded' && typeof item.height === 'number')
    await until(() => Boolean(document.activeElement?.classList.contains('canvas-text-editor')) && node(block).classList.contains('selected') && Boolean(control('Bold')), 'A new shape did not open for typing with its controls')
    await window.smoke.key('Escape')
    await until(() => !document.activeElement?.classList.contains('canvas-text-editor'), 'Escape did not leave the new shape')
    await new Promise((resolve) => setTimeout(resolve, 200))
    if (saved(block)?.text !== '' || !node(block)) throw new Error('Canvas format: a shape left without words did not stay on the board')
    const dot = () => node(block).querySelector('.canvas-next.right button')
    await until(() => Boolean(dot()), 'A selected shape did not show the dots that add the next one')
    await hover(dot())
    const ghost = () => node(block).querySelector('.canvas-ghost')?.getBoundingClientRect()
    await until(() => Boolean(ghost()) && Math.round(ghost()!.x - node(block).getBoundingClientRect().right) === Math.round(80 * boardZoom()), 'Pointing at a dot did not outline the next shape beside this one')
    const before = new Set(view.nodes.map((item) => item.id))
    await press(dot(), 'the dot on the right')
    const grown = () => view.nodes.find((item) => !before.has(item.id))
    await until(() => Boolean(grown()), 'A click on a dot did not add a shape')
    const next = grown()!
    const link = view.edges.find((edge) => edge.fromNode === block && edge.toNode === next.id)
    if (next.shape !== 'rounded' || next.x !== Number(saved(block).x) + 280 || next.y !== saved(block).y || next.width !== 200 || next.height !== 120 || link?.fromSide !== 'right' || link.toSide !== 'left') {
      throw new Error('Canvas format: the shape a dot added was not a copy beside the first, joined from its side: ' + JSON.stringify([next, link]))
    }
    await until(() => Boolean(document.activeElement?.classList.contains('canvas-text-editor')) && node(next.id).classList.contains('selected') && !node(block).querySelector('.canvas-next'), 'The added shape did not open for typing in place of the first')
    await window.smoke.key('k')
    await window.smoke.key('Escape')
    await until(() => saved(next.id).text === 'k', 'Words typed in a new shape were not saved')
    const tail = node(next.id).querySelector('.canvas-next.bottom button')!.getBoundingClientRect()
    const head = node(block).getBoundingClientRect()
    const pull = [[tail.x + tail.width / 2, tail.y + tail.height / 2], [head.x + head.width / 2, head.bottom]].map((point) => point.map(Math.round))
    const parts = view.nodes.length
    await window.smoke.pointer('mouseMove', pull[0][0], pull[0][1])
    await window.smoke.pointer('mouseDown', pull[0][0], pull[0][1])
    for (let step = 1; step <= 6; step++) {
      await window.smoke.pointer('mouseMove', Math.round(pull[0][0] + (pull[1][0] - pull[0][0]) * step / 6), Math.round(pull[0][1] + (pull[1][1] - pull[0][1]) * step / 6))
    }
    await window.smoke.pointer('mouseUp', pull[1][0], pull[1][1])
    await until(() => view.edges.some((edge) => edge.fromNode === next.id && edge.toNode === block && edge.fromSide === 'bottom' && edge.toSide === 'bottom'), 'A drag from a dot did not draw a connector from that side')
    if (view.nodes.length !== parts) throw new Error('Canvas format: a drag from a dot also added a shape')
    await press(element.querySelector('.react-flow__pane'), 'board')
    const hexagon = await place('Hexagon', [-110, 110, -70, 70], (item) => item.shape === 'hexagon' && typeof item.height === 'number', 'Shapes')
    await window.smoke.key('Escape')
    const again = await place('Shapes', [-110, 110, -70, 70], (item) => item.shape === 'hexagon' && item.id !== hexagon)
    await window.smoke.key('Escape')
    await until(() => node(again).classList.contains('selected') && !document.activeElement?.classList.contains('canvas-text-editor'), 'Escape did not leave the second hexagon')
    await press(element.querySelector('.react-flow__pane'), 'board')
    const table = await place('Table', [0, 430, 0, 140], (item) => item.type === 'table' && item.height === undefined, 'Shapes')
    await until(() => node(table)?.querySelectorAll('th, td').length === 9, 'A new table did not show its nine cells')
    if (element.querySelector('.canvas-banner')) throw new Error('Canvas format: the items just made were announced as changed by Jaz')
    if (['Table', 'Timer', 'Eraser'].some((tool) => dockTool(tool))) throw new Error('Canvas format: the dock still holds a tool that moved off it')
    const timer = 'timer'
    const face = () => node(timer).querySelector('.canvas-timer-face')!.textContent
    const timerButton = (label: string) => node(timer).querySelector(`[aria-label="${label}"]`)
    await until(() => face() === '5:00', 'A new timer did not show five minutes')
    await press(timerButton('One minute more'), 'One minute more')
    await until(() => saved(timer).seconds === 360 && face() === '6:00', 'The timer did not get longer')
    await press(timerButton('Start'), 'Start')
    await until(() => typeof saved(timer).endsAt === 'string' && face() === '5:59' && Boolean(timerButton('Pause')), 'The timer did not count down')
    await press(timerButton('Pause'), 'Pause')
    await until(() => saved(timer).endsAt === undefined && [358, 359, 360].includes(Number(saved(timer).left)) && Boolean(timerButton('Start')), 'The timer did not pause with its time left')
    await press(timerButton('Reset'), 'Reset')
    await until(() => saved(timer).left === undefined && face() === '6:00', 'The timer did not reset')
    await select(timer)
    await press(control('Comments'), 'Comments')
    const note = () => element.querySelector<HTMLInputElement>('.canvas-comments input')
    await until(() => Boolean(note()), 'Comments did not open')
    await press(note(), 'comment box')
    await until(() => document.activeElement === note(), 'The comment box did not take the keys')
    for (const key of 'ok?') {
      await window.smoke.key(key)
    }
    await window.smoke.key('Enter')
    await until(() => JSON.stringify(saved(timer).comments) === '[{"by":"user","text":"ok?"}]' && note()!.value === '' && element.querySelector('.canvas-comment-badge')?.textContent === '1', 'A comment was not pinned to the item')
    view = { ...applyPatch(view, { nodes: [{ id: timer, comments: [{ by: 'user', text: 'ok?' }, { by: 'agent', text: 'Yes, six minutes.' }] }] }), version: String(Number(view.version) + 1) }
    await client.invalidateQueries({ queryKey: keys.sessionCanvas(sessionId) })
    await until(() => element.querySelector('.canvas-comments p.agent')?.textContent === 'JazYes, six minutes.' && element.querySelector('.canvas-comment-badge')?.textContent === '2', 'The agent\'s reply did not show under the comment')
    await press([...element.querySelectorAll('.canvas-comments button')].find((button) => button.textContent === 'Resolve'), 'Resolve')
    await until(() => saved(timer).comments === undefined && !element.querySelector('.canvas-comment-badge'), 'Resolve did not clear the comments')
    await press(element.querySelector('.react-flow__pane'), 'board')
    const paste = async (text: string, check: (item: CanvasItem) => boolean, message: string) => {
      const data = new DataTransfer()
      data.setData('text/plain', text)
      element.querySelector('.react-flow__pane')!.dispatchEvent(new ClipboardEvent('paste', { clipboardData: data, bubbles: true, cancelable: true }))
      await until(() => check(view.nodes.at(-1)!) && node(view.nodes.at(-1)!.id)?.classList.contains('selected'), message)
    }
    await paste('Pasted from elsewhere', (item) => item.type === 'text' && item.text === 'Pasted from elsewhere' && item.height === undefined, 'Pasted words did not become a text item')
    await paste('https://example.com/plan', (item) => item.type === 'link' && item.url === 'https://example.com/plan', 'A pasted address did not become a page')
    await window.smoke.key('Backspace')
    await until(() => view.nodes.at(-1)!.type !== 'link', 'The pasted page was not removed')
    const drawing = document.createElement('canvas')
    drawing.width = 960
    drawing.height = 480
    drawing.getContext('2d')!.fillRect(0, 0, 960, 480)
    const png = await new Promise<Blob>((resolve) => drawing.toBlob((blob) => resolve(blob!), 'image/png'))
    const carried = (name: string) => {
      const data = new DataTransfer()
      data.items.add(new File([png], name, { type: 'image/png' }))
      return data
    }
    const pane = element.querySelector('.react-flow__pane')!
    pane.dispatchEvent(new ClipboardEvent('paste', { clipboardData: carried('pasted.png'), bubbles: true, cancelable: true }))
    await until(() => view.nodes.at(-1)!.file === 'uploads/pasted.png' && view.nodes.at(-1)!.width === 480 && view.nodes.at(-1)!.height === 240 && uploads.at(-1) === `pasted.png:${png.size}` && node(view.nodes.at(-1)!.id)?.classList.contains('selected'), 'A pasted picture was not uploaded and put on the board at its own shape')
    const spot = element.getBoundingClientRect()
    const over = pane.dispatchEvent(new DragEvent('dragover', { dataTransfer: carried('dropped.png'), bubbles: true, cancelable: true, clientX: spot.x + 300, clientY: spot.y + 300 }))
    pane.dispatchEvent(new DragEvent('drop', { dataTransfer: carried('dropped.png'), bubbles: true, cancelable: true, clientX: spot.x + 300, clientY: spot.y + 300 }))
    await until(() => !over && view.nodes.at(-1)!.file === 'uploads/dropped.png' && Math.abs(node(view.nodes.at(-1)!.id).getBoundingClientRect().x + node(view.nodes.at(-1)!.id).getBoundingClientRect().width / 2 - spot.x - 300) < 3, 'A dropped picture did not land where it was dropped')
    await window.smoke.key('Backspace')
    await until(() => view.nodes.at(-1)!.file === 'uploads/pasted.png', 'Delete did not remove the picture just dropped')
    await window.smoke.key('z', ['meta'])
    await until(() => view.nodes.at(-1)!.file === 'uploads/dropped.png', 'Undo did not bring the picture back')
    const pictures = view.nodes.filter((item) => String(item.file).startsWith('uploads/')).map((item) => item.id)
    view = { ...applyPatch(view, { remove: pictures }), version: String(Number(view.version) + 1) }
    await client.invalidateQueries({ queryKey: keys.sessionCanvas(sessionId) })
    await until(() => pictures.length === 2 && pictures.every((id) => !node(id)), 'The uploaded pictures were not cleared away')
    if (!dockTool('Picture or file')) {
      throw new Error('Canvas format: the dock has no upload tool')
    }
    const sticker = await place('Sticker', [-90, 90, -90, 90], (item) => item.text === '👍' && item.size === 'huge')
    await until(() => node(sticker).classList.contains('selected') && !document.activeElement?.classList.contains('canvas-text-editor') && dockTool('Select')?.getAttribute('aria-pressed') === 'true', 'A sticker was not put down ready to move')

    await press(node('counter'), 'counter card')
    await until(() => node('counter').classList.contains('selected') && !node('counter').querySelector('.canvas-shield'), 'The card did not take the pointer once selected')
    await press(node('counter').querySelector('iframe'), 'the card\'s button')
    await until(() => (saved('counter').data as { count: number }).count === 3, 'A card did not save its data on its item')
    await press(element.querySelector('.react-flow__pane'), 'board')
    await window.smoke.key('z', ['meta'])
    await until(() => (saved('counter').data as { count: number }).count === 2 && heard.at(-1) === 'heard 2', 'A card did not hear its data change under it')

    const draw = async (from: number[], to: number[]) => {
      const known = new Set(view.nodes.map((item) => item.id))
      await window.smoke.pointer('mouseMove', from[0], from[1])
      await window.smoke.pointer('mouseDown', from[0], from[1])
      for (let step = 1; step <= 8; step++) {
        await window.smoke.pointer('mouseMove', Math.round(from[0] + (to[0] - from[0]) * step / 8), Math.round(from[1] + (to[1] - from[1]) * step / 8))
      }
      await window.smoke.pointer('mouseUp', to[0], to[1])
      await until(() => view.nodes.some((item) => !known.has(item.id) && item.type === 'stroke'), 'The pen did not leave a line')
      return view.nodes.find((item) => !known.has(item.id))!.id
    }
    const board = element.getBoundingClientRect()
    const from = [Math.round(board.x + 90), Math.round(board.y + 150)]
    const to = [from[0] + 240, from[1] + 80]
    await press(dockTool('Pen'), 'Pen tool')
    const pen = () => element.querySelector('.canvas-tool-options[aria-label="Pen"]')
    await until(() => Boolean(pen()), 'The pen did not show its inks')
    const scribble = await draw(from, to)
    const zoom = boardZoom()
    const drawn = saved(scribble)
    if (Math.abs(Number(drawn.width) - 240 / zoom) > 3 || Math.abs(Number(drawn.height) - 80 / zoom) > 3 || (drawn.points as number[][]).length < 5 || drawn.color !== undefined) {
      throw new Error('Canvas format: the line was not saved where it was drawn: ' + JSON.stringify(drawn))
    }
    const ink = (id: string) => getComputedStyle(node(id).querySelector('.canvas-stroke path')!)
    await until(() => Math.abs(node(scribble).getBoundingClientRect().x - from[0]) < 3 && Math.abs(node(scribble).getBoundingClientRect().width - 240) < 4, 'The line was not shown where it was drawn')
    await press(pen()!.querySelector('[aria-label="Red"]'), 'Red ink')
    await press(pen()!.querySelector('[aria-label="Highlighter"]'), 'Highlighter')
    const marker = await draw([from[0], from[1] + 120], [to[0], from[1] + 120])
    await until(() => saved(marker).color === 'red' && saved(marker).highlight === true && ink(marker).opacity === '0.4' && ink(marker).stroke !== ink(scribble).stroke, 'The highlighter did not draw in its ink')
    if (dockTool('Pen')?.getAttribute('aria-pressed') !== 'true') {
      throw new Error('Canvas format: the pen was put down after a line')
    }
    await press(pen()!.querySelector('[aria-label="Straight line"]'), 'Straight line')
    const lines = view.nodes.length
    await window.smoke.pointer('mouseMove', from[0], from[1] + 200)
    await window.smoke.pointer('mouseDown', from[0], from[1] + 200)
    await window.smoke.pointer('mouseMove', from[0] + 60, from[1] + 280)
    await window.smoke.pointer('mouseMove', to[0], from[1] + 200)
    await window.smoke.pointer('mouseUp', to[0], from[1] + 200)
    await until(() => view.nodes.length === lines + 1 && (view.nodes.at(-1)!.points as number[][]).length === 2 && view.nodes.at(-1)!.height === 1, 'A straight line kept the wander of the pointer')
    await press(dockTool('Select'), 'Select tool')
    await window.smoke.pointer('mouseMove', from[0] + 120, from[1] + 40)
    await window.smoke.pointer('mouseDown', from[0] + 120, from[1] + 40)
    await window.smoke.pointer('mouseUp', from[0] + 120, from[1] + 40)
    await until(() => node(scribble).classList.contains('selected') && Boolean(control('Ink')), 'A click on the ink did not select the line')
    await pick('Ink', 'Blue')
    await until(() => saved(scribble).color === 'blue', 'The line did not take a new ink')
    await press(control('Weight: 4'), 'Weight')
    await until(() => saved(scribble).weight === 8 && ink(scribble).strokeWidth === '8px', 'The line did not take a new weight')
    await new Promise((resolve) => setTimeout(resolve, 500))
    await window.smoke.capture('canvas-format-edited')
    await press(dockTool('Pen'), 'Pen tool')
    await until(() => Boolean(pen()), 'The pen did not show its options again')
    await press(pen()!.querySelector('[aria-label="Eraser"]'), 'the pen\'s eraser')
    await until(() => Boolean(element.querySelector('.canvas-eraser-layer')), 'The pen\'s eraser did not take over the board')
    await window.smoke.pointer('mouseMove', from[0] + 120, from[1] + 100)
    await window.smoke.pointer('mouseDown', from[0] + 120, from[1] + 100)
    await window.smoke.pointer('mouseMove', from[0] + 120, from[1] + 70)
    await window.smoke.pointer('mouseMove', from[0] + 120, from[1] + 40)
    await window.smoke.pointer('mouseMove', from[0] + 120, from[1] + 20)
    await window.smoke.pointer('mouseUp', from[0] + 120, from[1] + 20)
    await until(() => !view.nodes.some((item) => item.id === scribble) && view.nodes.some((item) => item.id === marker), 'The eraser did not remove the line it crossed, and only that scribble')
    await press(dockTool('Select'), 'Select tool')
  } finally {
    root.unmount()
    client.clear()
    element.remove()
    window.fetch = originalFetch
  }
}
