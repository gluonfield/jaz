import { describe, expect, test } from 'bun:test'
import {
  SESSION_PAGE_SIZE,
  applySidebarProjectOrder,
  applySessionDragOrder,
  sessionDisplayBlocks,
  sessionPage,
  sidebarSessionSections,
} from './sidebarSessionModel'

function item(id, { path = '', pinned = false, at = id } = {}) {
  return {
    child: false,
    session: {
      id,
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

  test('sorts the flat list by attention time instead of project or parent order', () => {
    const sections = sidebarSessionSections([
      item('older', { at: '2026-08-15T12:00:00Z' }),
      item('newest', { at: '2026-08-17T12:00:00Z' }),
      item('middle', { at: '2026-08-16T12:00:00Z' }),
    ], [])

    expect(sections.recentItems.map((entry) => entry.session.id)).toEqual([
      'newest',
      'middle',
      'older',
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

  test('preserves project drag order around the fixed no-project block', () => {
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
