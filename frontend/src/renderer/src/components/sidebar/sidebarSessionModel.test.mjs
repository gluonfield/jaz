import { describe, expect, test } from 'bun:test'
import {
  SESSION_PAGE_SIZE,
  applySidebarProjectOrder,
  applySessionDragOrder,
  sessionDisplayBlocks,
  sessionPage,
  sidebarSessionSections,
} from './sidebarSessionModel'

function item(id, { path = '', pinned = false, parent, at = id } = {}) {
  return {
    child: Boolean(parent),
    session: {
      id,
      parent_id: parent,
      pinned,
      runtime_ref: path ? { project_path: path } : undefined,
      updated_at: at,
      last_attention_at: at,
    },
  }
}

describe('sidebar session organization', () => {
  test('shows fifty recent sessions before exposing the remainder', () => {
    const sessions = Array.from({ length: 55 }, (_, index) => item(`session-${index}`))

    const page = sessionPage(sessions, SESSION_PAGE_SIZE)

    expect(page.items).toHaveLength(50)
    expect(page.total).toBe(55)
  })

  test('keeps each recent child under its own parent despite interleaved attention times', () => {
    const sections = sidebarSessionSections([
      item('rust', { at: '2026-11-21T04:02:00Z' }),
      item('rust-child', { parent: 'rust', at: '2026-11-21T04:14:00Z' }),
      item('other', { at: '2026-11-21T04:06:00Z' }),
      item('copilot', { at: '2026-11-13T11:38:00Z' }),
      item('copilot-child', { parent: 'copilot', at: '2026-11-21T04:05:00Z' }),
    ], [])

    expect(sections.recentItems.map(({ session, child }) => [session.id, child])).toEqual([
      ['rust', false],
      ['rust-child', true],
      ['other', false],
      ['copilot', false],
      ['copilot-child', true],
    ])
  })

  test('keeps project expansion independent from the no-project page', () => {
    const projectPath = 'ungrouped'
    const projectSessions = Array.from(
      { length: 55 },
      (_, index) => item(`project-${index}`, { path: projectPath }),
    )
    const ungroupedSessions = Array.from(
      { length: 55 },
      (_, index) => item(`ungrouped-${index}`),
    )
    const sections = sidebarSessionSections(
      [item('pinned', { pinned: true }), ...projectSessions, ...ungroupedSessions],
      [{ path: projectPath, name: 'Named project' }],
    )

    const blocks = sessionDisplayBlocks(
      sections.groups,
      sections.ungrouped,
      new Set([projectPath]),
      false,
      new Set(),
    )
    const project = blocks.find((block) => block.kind === 'project')
    const ungrouped = blocks.find((block) => block.kind === 'ungrouped')

    expect(sections.pinnedItems.map((entry) => entry.session.id)).toEqual(['pinned'])
    expect(project?.items).toHaveLength(55)
    expect(ungrouped?.items).toHaveLength(5)
    expect(new Set(blocks.map((block) => block.key)).size).toBe(blocks.length)
  })

  test('shows five conversations per folder and expands only the requested folder', () => {
    const groups = ['physicslab', 'jaz'].map((key) => ({
      key,
      label: key,
      items: Array.from({ length: 12 }, (_, index) => item(`${key}-${index}`)),
    }))
    const initial = sessionDisplayBlocks(groups, [], new Set(), false, new Set())

    expect(initial.map((block) => block.items.length)).toEqual([5, 5])
    expect(initial[0].items.map((entry) => entry.session.id)).toEqual([
      'physicslab-0', 'physicslab-1', 'physicslab-2', 'physicslab-3', 'physicslab-4',
    ])

    const expanded = sessionDisplayBlocks(groups, [], new Set(['physicslab']), false, new Set())
    expect(expanded.map((block) => block.items.length)).toEqual([12, 5])
  })

  test.each([
    '2026-10-07T12:00:00Z',
    '2026-10-05T12:00:00Z',
    '2026-10-01T12:00:00Z',
  ])('keeps every project above Recents with ungrouped activity at %s', (at) => {
    const sections = sidebarSessionSections([
      item('alpha', { path: '/alpha', at: '2026-10-06T12:00:00Z' }),
      item('beta', { path: '/beta', at: '2026-10-04T12:00:00Z' }),
      item('recent', { at }),
    ], [
      { path: '/alpha', name: 'Alpha' },
      { path: '/beta', name: 'Beta' },
    ])

    for (const order of [null, ['project:/beta', 'ungrouped', 'project:/alpha']]) {
      const groups = applySessionDragOrder(sections.groups, order)
      const blocks = sessionDisplayBlocks(groups, sections.ungrouped, new Set(), false, new Set())

      expect(blocks.map((block) => block.key)).toEqual(
        order ? ['project:/beta', 'project:/alpha', 'ungrouped'] : ['project:/alpha', 'project:/beta', 'ungrouped'],
      )
    }
  })

  test('preserves project drag order independently of the no-project block', () => {
    const groups = [
      { key: '/alpha', label: 'Alpha', items: [] },
      { key: '/beta', label: 'Beta', items: [] },
    ]

    const ordered = applySessionDragOrder(groups, ['project:/beta', 'ungrouped', 'project:/alpha'])

    expect(ordered.map((group) => group.key)).toEqual(['/beta', '/alpha'])
  })

  test('reorders visible projects without moving projects absent from the sidebar', () => {
    const projects = [
      { path: '/alpha', name: 'Alpha' },
      { path: '/hidden', name: 'Hidden' },
      { path: '/beta', name: 'Beta' },
    ]

    const ordered = applySidebarProjectOrder(
      projects,
      ['project:/beta', 'ungrouped', 'project:/alpha'],
    )

    expect(ordered.map((project) => project.path)).toEqual(['/beta', '/hidden', '/alpha'])
  })
})


test('shows project groups only when they contain visible unpinned sessions', () => {
  const projects = [
    { path: '/empty', name: 'Empty' },
    { path: '/pinned', name: 'Pinned only' },
    { path: '/active', name: 'Active' },
  ]
  const sessions = [item('pinned', { path: '/pinned', pinned: true }), item('thread', { path: '/active' })]
  const sections = sidebarSessionSections(sessions, projects)
  expect(sections.groups.map((group) => [group.key, group.items.length])).toEqual([
    ['/active', 1],
  ])
  const blocks = sessionDisplayBlocks(sections.groups, [], new Set(), false, new Set())
  expect(blocks.map((block) => block.key)).toEqual(['project:/active'])
  expect(sections.pinnedItems.map((entry) => entry.session.id)).toEqual(['pinned'])
  expect(sidebarSessionSections([], projects).groups).toEqual([])

  const removed = sidebarSessionSections(sessions, projects.filter((project) => project.path !== '/active'))
  expect(removed.groups).toEqual([])
  expect(removed.ungrouped.map((entry) => entry.session.id)).toEqual(['thread'])
})
