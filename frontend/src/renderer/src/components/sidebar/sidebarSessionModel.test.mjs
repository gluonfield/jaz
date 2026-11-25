import { describe, expect, test } from 'bun:test'
import {
  SESSION_PAGE_SIZE,
  applySidebarProjectOrder,
  applySessionDragOrder,
  sessionDisplayBlocks,
  sessionPage,
  sidebarSessionSections,
} from './sidebarSessionModel'

function session(id, { path = '', pinned = false, parent, at = '2026-11-21T04:00:00Z' } = {}) {
  return {
    id,
    parent_id: parent,
    pinned,
    runtime_ref: path ? { project_path: path } : undefined,
    updated_at: at,
    last_attention_at: at,
  }
}

describe('sidebar session organization', () => {
  test('shows fifty recent sessions before exposing the remainder', () => {
    const sessions = Array.from({ length: 55 }, (_, index) => session(`session-${index}`))
    const { recentItems } = sidebarSessionSections(sessions, [])

    const page = sessionPage(recentItems, SESSION_PAGE_SIZE)

    expect(page.items).toHaveLength(50)
    expect(page.total).toBe(55)
  })

  test('keeps each recent child under its own parent despite interleaved attention times', () => {
    const sections = sidebarSessionSections([
      session('other', { at: '2026-11-21T04:06:00Z' }),
      session('copilot-child', { parent: 'copilot', at: '2026-11-21T04:05:00Z' }),
      session('rust-child', { parent: 'rust', at: '2026-11-21T04:14:00Z' }),
      session('copilot', { at: '2026-11-13T11:38:00Z' }),
      session('rust', { at: '2026-11-21T04:02:00Z' }),
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
      (_, index) => session(`project-${index}`, { path: projectPath }),
    )
    const ungroupedSessions = Array.from(
      { length: 55 },
      (_, index) => session(`ungrouped-${index}`),
    )
    const sections = sidebarSessionSections(
      [session('pinned', { pinned: true }), ...projectSessions, ...ungroupedSessions],
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
      items: Array.from({ length: 12 }, (_, index) => ({ session: session(`${key}-${index}`), child: false })),
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
      session('alpha', { path: '/alpha', at: '2026-10-06T12:00:00Z' }),
      session('beta', { path: '/beta', at: '2026-10-04T12:00:00Z' }),
      session('recent', { at }),
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
  const sessions = [session('pinned', { path: '/pinned', pinned: true }), session('thread', { path: '/active' })]
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

describe('unified cloud & local sidebar grouping, sorting, and filtering', () => {
  test('groups Cloud projects and Local projects in the same Project view while preserving empty Cloud projects', () => {
    const spaces = [
      { id: 'sp_1', name: 'rockstars', visibility: 'team' },
      { id: 'sp_empty', name: 'Q4', visibility: 'private' },
    ]
    const localProjects = [{ path: '/repo/jaz', name: 'jaz' }]
    const sessions = [
      { id: 'c1', space_id: 'sp_1', backend: 'cloud', updated_at: '2026-11-25T09:00:00Z', last_attention_at: '2026-11-25T09:00:00Z' },
      { id: 'c_loose', backend: 'cloud', updated_at: '2026-11-25T08:00:00Z', last_attention_at: '2026-11-25T08:00:00Z' },
      { id: 'l1', runtime_ref: { project_path: '/repo/jaz' }, updated_at: '2026-11-25T07:00:00Z', last_attention_at: '2026-11-25T07:00:00Z' },
    ]
    const sections = sidebarSessionSections(sessions, localProjects, { spaces, groupBy: 'project' })
    expect(sections.groups.map((g) => [g.key, g.kind, g.items.map((i) => i.session.id)])).toEqual([
      ['sp_1', 'cloud', ['c1']],
      ['sp_empty', 'cloud', []],
      ['/repo/jaz', 'local', ['l1']],
    ])
    expect(sections.ungrouped.map((i) => i.session.id)).toEqual(['c_loose'])
  })

  test('supports Date, Status, Environment grouping, Created sorting, and Show filters', () => {
    const now = new Date('2026-11-25T12:00:00Z')
    const sessions = [
      { id: 'run', status: 'running', backend: 'cloud', created_at: '2026-11-20T10:00:00Z', updated_at: '2026-11-25T11:00:00Z', last_attention_at: '2026-11-25T11:00:00Z' },
      { id: 'unread', status: 'idle', unread: true, backend: 'local', created_at: '2026-11-24T10:00:00Z', updated_at: '2026-11-24T11:00:00Z', last_attention_at: '2026-11-24T11:00:00Z' },
      { id: 'old', status: 'idle', backend: 'cloud', created_at: '2026-10-01T10:00:00Z', updated_at: '2026-10-01T10:00:00Z', last_attention_at: '2026-10-01T10:00:00Z' },
    ]

    const byDate = sidebarSessionSections(sessions, [], { groupBy: 'date', now })
    expect(byDate.groups.map((g) => [g.label, g.items.map((i) => i.session.id)])).toEqual([
      ['Today', ['run']],
      ['Yesterday', ['unread']],
      ['Older', ['old']],
    ])

    const byStatus = sidebarSessionSections(sessions, [], { groupBy: 'status' })
    expect(byStatus.groups.map((g) => [g.label, g.items.map((i) => i.session.id)])).toEqual([
      ['Running', ['run']],
      ['Unread', ['unread']],
      ['Idle', ['old']],
    ])

    const byEnv = sidebarSessionSections(sessions, [], { groupBy: 'environment' })
    expect(byEnv.groups.map((g) => [g.label, g.items.map((i) => i.session.id)])).toEqual([
      ['Cloud', ['run', 'old']],
      ['Local', ['unread']],
    ])

    const byCreated = sidebarSessionSections(sessions, [], { groupBy: 'none', sortBy: 'created' })
    expect(byCreated.recentItems.map((i) => i.session.id)).toEqual(['unread', 'run', 'old'])

    const onlyUnread = sidebarSessionSections(sessions, [], { groupBy: 'none', show: 'unread' })
    expect(onlyUnread.recentItems.map((i) => i.session.id)).toEqual(['unread'])

    const onlyActive = sidebarSessionSections(sessions, [], { groupBy: 'none', show: 'active' })
    expect(onlyActive.recentItems.map((i) => i.session.id)).toEqual(['run', 'unread'])
  })
})
