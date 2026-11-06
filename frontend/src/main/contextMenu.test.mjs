import { expect, test } from 'bun:test'
import { build } from 'bun'
import { EventEmitter } from 'node:events'
import { createRequire } from 'node:module'

const bundle = await build({ entrypoints: [new URL('./contextMenu.ts', import.meta.url).pathname], target: 'node', format: 'cjs', external: ['electron'] })
if (!bundle.success) throw new AggregateError(bundle.logs)
const evaluate = new Function('require', 'module', 'exports', await bundle.outputs[0].text())
const require = createRequire(import.meta.url)

function fixture() {
  const contents = new EventEmitter()
  const copiedImages = []
  const copiedPaths = []
  const opened = []
  const menus = []
  const window = {}
  let backend = 'https://backend.example'
  contents.copyImageAt = (x, y) => copiedImages.push({ x, y })
  const electron = {
    BrowserWindow: { fromWebContents: () => window },
    Menu: { buildFromTemplate: items => ({ popup: options => menus.push({ items, options }) }) },
    clipboard: { writeText: text => copiedPaths.push(text) },
    shell: { openExternal: url => opened.push(url) },
  }
  const module = { exports: {} }
  evaluate(name => name === 'electron' ? electron : require(name), module, module.exports)
  module.exports.attachContextMenu(contents, {
    canOpenPreviewURL: () => true,
    openPreviewURL: url => opened.push(url),
    backendOrigin: () => backend,
  })
  const show = (params = {}) => {
    contents.emit('context-menu', {}, {
      x: 125, y: 240, linkURL: '', isEditable: false, selectionText: '', mediaType: 'none',
      srcURL: '', hasImageContents: false, editFlags: { canCopy: false }, ...params,
    })
    return menus.at(-1)?.items ?? []
  }
  return { show, copiedImages, copiedPaths, opened, menus, window, backend: value => { backend = value } }
}

test('image actions copy the clicked image and its source path without backend credentials', () => {
  const f = fixture()
  for (const path of ['/srv/charts/heat map #1.png', 'C:\\work\\charts\\图.png', 'docs/../chart.png', 'file:///srv/chart.png']) {
    const source = new URL('https://backend.example/v1/sessions/session-1/file')
    source.search = new URLSearchParams({ path, raw: '1', ticket: 'private-resource-ticket', image: 'cache-identity' })
    const items = f.show({ mediaType: 'image', hasImageContents: true, srcURL: source.href })
    expect(items.map(item => item.label)).toEqual(['Copy Image', 'Copy Path'])
    expect(items[0].enabled).toBe(true)
    items[0].click()
    items[1].click()
    expect(f.copiedImages.at(-1)).toEqual({ x: 125, y: 240 })
    expect(f.copiedPaths.at(-1)).toBe(path)
    expect(f.menus.at(-1).options.window).toBe(f.window)
  }
})

test('external image URLs keep their query and only the current backend resolves session paths', () => {
  const f = fixture()
  const sources = [
    'https://images.example/chart.png?version=2&path=original.png#plot',
    'https://images.example/v1/sessions/s/file?path=original.png&raw=1',
    'https://backend.example/other-image.png?path=original.png',
    'data:image/png;base64,aGVsbG8=',
  ]
  for (const srcURL of sources) {
    f.show({ mediaType: 'image', srcURL })[1].click()
    expect(f.copiedPaths.at(-1)).toBe(srcURL)
  }
  f.backend('https://new-backend.example')
  f.show({ mediaType: 'image', srcURL: 'https://new-backend.example/v1/sessions/s/file?path=%2Fsrv%2Fchart.png&ticket=private' })[1].click()
  expect(f.copiedPaths.at(-1)).toBe('/srv/chart.png')
})

test('unloaded images disable image copy and missing sources disable path copy', () => {
  const f = fixture()
  const broken = f.show({ mediaType: 'image', srcURL: 'https://images.example/missing.png' })
  expect(broken[0].enabled).toBe(false)
  expect(broken[1].enabled).toBe(true)
  const missing = f.show({ mediaType: 'image' })
  expect(missing[0].enabled).toBe(false)
  expect(missing[1].enabled).toBe(false)
})

test('backend attachment URLs omit resource credentials and open menus retain their source path', () => {
  const f = fixture()
  f.show({ mediaType: 'image', srcURL: 'https://backend.example/v1/sessions/s/attachments/a?ticket=private&image=cache' })[1].click()
  expect(f.copiedPaths.at(-1)).toBe('https://backend.example/v1/sessions/s/attachments/a')
  const items = f.show({ mediaType: 'image', srcURL: 'https://backend.example/v1/sessions/s/file?path=%2Fsrv%2Fchart.png&ticket=private' })
  f.backend('https://new-backend.example')
  items[1].click()
  expect(f.copiedPaths.at(-1)).toBe('/srv/chart.png')
})

test('linked images retain link actions and selected text retains its copy action', () => {
  const f = fixture()
  const url = 'https://example.com/report'
  const items = f.show({ mediaType: 'image', srcURL: 'https://images.example/chart.png', linkURL: url, selectionText: 'Chart', editFlags: { canCopy: true } })
  expect(items.slice(0, 3).map(item => item.label ?? item.type)).toEqual(['Copy Image', 'Copy Path', 'separator'])
  items.find(item => item.label === 'Open in Side Browser').click()
  items.find(item => item.label === 'Open in Browser').click()
  expect(f.opened).toEqual([url, url])
  expect(items.at(-1)).toEqual({ role: 'copy', enabled: true })
  expect(f.show({ selectionText: 'Text', editFlags: { canCopy: true } }).at(-1)).toEqual({ role: 'copy', enabled: true })
})

test('blank space has no menu and editable controls retain standard actions', () => {
  const f = fixture()
  expect(f.show()).toEqual([])
  expect(f.menus).toEqual([])
  expect(f.show({ isEditable: true, editFlags: { canCut: true, canCopy: true, canPaste: false, canSelectAll: true } })).toEqual([
    { role: 'cut', enabled: true },
    { role: 'copy', enabled: true },
    { role: 'paste', enabled: false },
    { role: 'selectAll', enabled: true },
  ])
})
