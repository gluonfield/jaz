import { expect, test } from 'bun:test'
import { pluginViewTab, refreshPluginTabs, sidePanelTabs } from '@/lib/sidePanelTabs'

const empty = { tabs: [], activeId: null }
const open = (state, tab) => sidePanelTabs(state, { type: 'open', tab })

test('files replace the empty picker, reuse a path, and preserve separate documents', () => {
  let state = open(empty, { id: 'file', kind: 'file', file: null })
  state = open(state, { id: 'file:a', kind: 'file', file: { path: '/a.md', location: { line: 1 } } })
  state = open(state, { id: 'file:b', kind: 'file', file: { path: '/b.md' } })
  state = open(state, { id: 'file:a', kind: 'file', file: { path: '/a.md', location: { line: 42 } } })
  expect(state.tabs).toHaveLength(2)
  expect(state.tabs[0].file.location.line).toBe(42)
  expect(state.tabs[1].file.path).toBe('/b.md')
  expect(state.activeId).toBe('file:a')
})

test('closing a tab selects its right neighbour, then left, then the empty state', () => {
  let state = empty
  for (const id of ['one', 'two', 'three']) {
    state = open(state, { id, kind: 'preview' })
  }
  state = sidePanelTabs(state, { type: 'select', id: 'two' })
  state = sidePanelTabs(state, { type: 'close', id: 'two' })
  expect(state.activeId).toBe('three')
  state = sidePanelTabs(state, { type: 'close', id: 'three' })
  expect(state.activeId).toBe('one')
  state = sidePanelTabs(state, { type: 'close', id: 'one' })
  expect(state).toEqual(empty)
})

test('closing an inactive tab keeps selection and stale actions cannot select a missing tab', () => {
  let state = open(empty, { id: 'terminal', kind: 'terminal' })
  state = open(state, { id: 'browser', kind: 'preview' })
  state = sidePanelTabs(state, { type: 'close', id: 'terminal' })
  expect(state.activeId).toBe('browser')
  expect(sidePanelTabs(state, { type: 'select', id: 'terminal' })).toBe(state)
  expect(sidePanelTabs(state, { type: 'close', id: 'terminal' })).toBe(state)
})

test('reordering retains selection, file locations, and hidden tabs', () => {
  let state = open(empty, { id: 'file', kind: 'file', file: { path: '/notes.md', location: { line: 12 } } })
  state = open(state, { id: 'side-chat', kind: 'side-chat' })
  state = open(state, { id: 'browser', kind: 'preview' })
  const reordered = sidePanelTabs(state, { type: 'reorder', ids: ['browser', 'file'] })
  expect(reordered.tabs.map((tab) => tab.id)).toEqual(['browser', 'file', 'side-chat'])
  expect(reordered.activeId).toBe('browser')
  expect(reordered.tabs[1]).toBe(state.tabs[0])
  expect(state.tabs.map((tab) => tab.id)).toEqual(['file', 'side-chat', 'browser'])
  expect(sidePanelTabs(reordered, { type: 'close', id: 'browser' }).activeId).toBe('file')
})

test('tabs opened beside another tab land to its right and fall back to the end', () => {
  let state = empty
  for (const id of ['one', 'two', 'three']) {
    state = open(state, { id, kind: 'preview' })
  }
  state = sidePanelTabs(state, { type: 'open', tab: { id: 'copy', kind: 'preview' }, after: 'one' })
  state = sidePanelTabs(state, { type: 'open', tab: { id: 'orphan', kind: 'preview' }, after: 'closed' })
  expect(state.tabs.map((tab) => tab.id)).toEqual(['one', 'copy', 'two', 'three', 'orphan'])
  expect(state.activeId).toBe('orphan')
})

test('refetched plugin HTML updates mounted tabs without disturbing selection, order, or closed tabs', () => {
  const saved = { reference: 'example/status', file: 'status.json', title: 'Status', widget_code: '<p>Version 1</p>' }
  const standalone = { file: saved.file, title: 'Standalone', widget_code: '<p>Standalone</p>' }
  const closed = { ...saved, file: 'closed.json' }
  const terminal = { id: 'terminal', kind: 'terminal' }
  const openTabs = [pluginViewTab(saved), pluginViewTab(standalone), terminal, pluginViewTab(closed)]
  let state = sidePanelTabs(empty, { type: 'restore', tabs: openTabs })
  state = sidePanelTabs(state, { type: 'select', id: terminal.id })
  state = sidePanelTabs(state, { type: 'close', id: pluginViewTab(closed).id })

  const current = { ...saved, widget_code: '<p>Version 2</p>' }
  const tabs = refreshPluginTabs(state.tabs, [current, { ...standalone, widget_code: '<p>Another snapshot</p>' }, closed])
  expect(tabs.map((tab) => tab.id)).toEqual(state.tabs.map((tab) => tab.id))
  expect(tabs.find((tab) => tab.id === state.activeId)).toBe(terminal)
  expect(tabs[0].view).toBe(current)
  expect(tabs[1]).toBe(openTabs[1])
})
