import { expect, test } from 'bun:test'
import { build } from 'bun'
import { createRequire } from 'node:module'
import * as React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'

const bundle = await build({
  entrypoints: [new URL('./SidebarWorkspaceSwitcher.tsx', import.meta.url).pathname],
  target: 'browser',
  format: 'cjs',
  external: [
    'react',
    'react/jsx-runtime',
    'react/jsx-dev-runtime',
    '@tanstack/react-router',
    'lucide-react',
    '@/components/connection/AddCloudWorkspaceModal',
    '@/components/sidebar/workspaceSwitchPassage',
    '@/components/ui/Popover',
    '@/components/ui/toast',
    '@/lib/api/client',
    '@/lib/clientRuntime',
    '@/lib/cloudAccount',
    '@/lib/cloudSession',
  ],
  define: { 'import.meta.env': '{}' },
})
if (!bundle.success) {
  throw new AggregateError(bundle.logs)
}
const evaluate = new Function('require', 'module', 'exports', await bundle.outputs[0].text())
const require = createRequire(import.meta.url)

function loadSwitcher(
  cloudState,
  {
    localBackend = true,
    forcePopoverOpen = false,
    pathname = '/new',
    search = {},
    onSwitchWorkspace = async () => null,
    onTurn = () => ({ shut: Promise.resolve(), arrive: async () => {}, cancel: () => {} }),
    onNavigate = () => {},
    adding = false,
    onSetAdding = () => {},
  } = {},
) {
  const mod = { exports: {} }
  evaluate(
    (name) => {
      if (name === 'react') {
        let stateCall = 0
        return {
          ...React,
          useRef: (initial) => ({ current: initial }),
          useState: (initial) => {
            const idx = stateCall++
            if (idx === 0 && forcePopoverOpen) return [true, () => {}]
            if (idx === 2) return [adding, onSetAdding]
            return [typeof initial === 'function' ? initial() : initial, () => {}]
          },
        }
      }
      if (name === '@tanstack/react-router') {
        return {
          useNavigate: () => onNavigate,
          useRouterState: ({ select }) => select({ location: { pathname, search } }),
        }
      }
      if (name === '@/components/connection/AddCloudWorkspaceModal') {
        return {
          AddCloudWorkspaceModal: ({ open }) =>
            open ? React.createElement('div', { 'data-add-cloud-modal': 'open' }) : null,
        }
      }
      if (name === '@/components/sidebar/workspaceSwitchPassage') {
        return { turnToWorkspace: onTurn }
      }
      if (name === '@/components/ui/Popover') {
        return {
          MenuRow: ({ role, selected, onClick, children }) =>
            React.createElement('button', { role, 'aria-checked': selected, onClick }, children),
          Popover: ({ open, trigger, children }) =>
            React.createElement(
              React.Fragment,
              null,
              trigger,
              open ? React.createElement('div', { 'data-popover': 'open' }, children) : null,
            ),
        }
      }
      if (name === '@/components/ui/toast') {
        return {
          useToast: () => () => {},
        }
      }
      if (name === '@/lib/api/client') {
        return {
          apiEmbeddedGetUrl: (path) => `https://backend.example${path}&ticket=t`,
        }
      }
      if (name === '@/lib/clientRuntime') {
        return {
          clientRuntime: { capabilities: { localBackend } },
        }
      }
      if (name === '@/lib/cloudAccount') {
        return {
          LOCAL_WORKSPACE_ID: 'local',
          switchWorkspace: onSwitchWorkspace,
        }
      }
      if (name === '@/lib/cloudSession') {
        return {
          useCloudSession: () => cloudState,
        }
      }
      return require(name)
    },
    mod,
    mod.exports,
  )
  return mod.exports.SidebarWorkspaceSwitcher
}

const exampleWorkspace = {
  id: 'ws_example',
  url: 'https://workspaces.example.com',
  org_id: 'org_example',
  org_name: 'Example Systems',
  email: 'alex@example.com',
  display_name: 'Alex',
  role: 'admin',
}

const personalWorkspace = {
  id: 'ws_personal',
  url: 'https://personal.jaz.example',
  org_id: 'org_personal',
  org_name: 'Personal Cloud',
  email: 'me@personal.example',
  display_name: 'Me',
  role: 'member',
}

const noWorkspaces = {
  active: false,
  source: null,
  status: 'disconnected',
  activeWorkspaceId: 'local',
  workspaces: [],
  url: '',
  user: null,
  error: null,
  oidcLoginUrl: '',
  capabilities: {},
  lastSyncAt: '',
}

// The menu's workspace rows, in order.
function workspaceRows(Switcher) {
  const rows = []
  const walk = (node) => {
    if (!node || typeof node !== 'object') return
    if (Array.isArray(node)) return node.forEach(walk)
    if (node.props?.role === 'menuitemradio') rows.push(node)
    walk(node.props?.trigger)
    walk(node.props?.children)
  }
  walk(Switcher({}))
  return rows
}

const settled = () => new Promise((resolve) => setTimeout(resolve, 0))

// The text of the menu row marked as the current workspace.
function checkedRow(html) {
  return html.match(/aria-checked="true">(.*?)<\/button>/)?.[1].replace(/<[^>]+>/g, '')
}

test('SidebarWorkspaceSwitcher stays out of the main sidebar until a Cloud workspace is connected', () => {
  const Switcher = loadSwitcher(noWorkspaces, { localBackend: true, forcePopoverOpen: true })
  expect(renderToStaticMarkup(React.createElement(Switcher))).toBe('')
})

test('SidebarWorkspaceSwitcher offers Local and Add Cloud Workspace in Settings before any Cloud workspace exists', () => {
  const Switcher = loadSwitcher(noWorkspaces, { localBackend: true, forcePopoverOpen: true })
  const html = renderToStaticMarkup(React.createElement(Switcher, { showWithoutCloud: true }))
  expect(html).toContain('aria-label="Workspace: Local"')
  expect(checkedRow(html)).toBe('Local')
  expect(html).toContain('Add Cloud Workspace')
})

test('SidebarWorkspaceSwitcher renders nothing when Local is disabled and no Cloud workspace is connected', () => {
  const Switcher = loadSwitcher(
    { ...noWorkspaces, source: 'primary', activeWorkspaceId: '' },
    { localBackend: false, forcePopoverOpen: true },
  )
  expect(renderToStaticMarkup(React.createElement(Switcher, { showWithoutCloud: true }))).toBe('')
})

test('SidebarWorkspaceSwitcher lists Cloud workspaces, Local, and Add Cloud Workspace as single-line rows when localBackend=true', () => {
  const cloudActiveHtml = renderToStaticMarkup(
    React.createElement(
      loadSwitcher(
        {
          active: true,
          source: 'attached',
          status: 'connected',
          activeWorkspaceId: 'ws_example',
          workspaces: [exampleWorkspace, personalWorkspace],
          url: exampleWorkspace.url,
          user: { id: 'u1', org_id: 'org_example', org_name: 'Example Systems', email: 'alex@example.com', display_name: 'Alex', role: 'admin' },
          error: null,
          oidcLoginUrl: '/v1/auth/login',
          capabilities: { spaces: true },
          lastSyncAt: '',
        },
        { localBackend: true, forcePopoverOpen: true },
      ),
    ),
  )

  expect(cloudActiveHtml).toContain('aria-label="Workspace: Example Systems"')
  expect(cloudActiveHtml).toContain('Example Systems')
  expect(cloudActiveHtml).toContain('Personal Cloud')
  expect(cloudActiveHtml).toContain('Local')
  expect(cloudActiveHtml).toContain('Add Cloud Workspace')
  expect(cloudActiveHtml).not.toContain('alex@example.com')
  expect(cloudActiveHtml).not.toContain('workspaces.example.com')
  expect(cloudActiveHtml).not.toContain('This computer')
  expect(checkedRow(cloudActiveHtml)).toBe('EExample Systems')

  const localActiveHtml = renderToStaticMarkup(
    React.createElement(
      loadSwitcher(
        {
          active: false,
          source: 'attached',
          status: 'disconnected',
          activeWorkspaceId: 'local',
          workspaces: [exampleWorkspace],
          url: '',
          user: null,
          error: null,
          oidcLoginUrl: '',
          capabilities: {},
          lastSyncAt: '',
        },
        { localBackend: true, forcePopoverOpen: true },
      ),
    ),
  )
  expect(localActiveHtml).toContain('aria-label="Workspace: Local"')
  expect(localActiveHtml).toContain('Example Systems')
  expect(checkedRow(localActiveHtml)).toBe('Local')
})

test('SidebarWorkspaceSwitcher NEVER shows Local in cloud-only mode (localBackend=false)', () => {
  // Single primary Cloud workspace: clean static header, no dropdown, no Local.
  const singleCloudHtml = renderToStaticMarkup(
    React.createElement(
      loadSwitcher(
        {
          active: true,
          source: 'primary',
          status: 'connected',
          activeWorkspaceId: 'ws_example',
          workspaces: [exampleWorkspace],
          url: exampleWorkspace.url,
          user: { id: 'u1', org_id: 'org_example', org_name: 'Example Systems', email: 'alex@example.com', display_name: 'Alex', role: 'admin' },
          error: null,
          oidcLoginUrl: '/v1/auth/login',
          capabilities: { spaces: true },
          lastSyncAt: '',
        },
        { localBackend: false, forcePopoverOpen: true },
      ),
    ),
  )
  expect(singleCloudHtml).toContain('Example Systems')
  expect(singleCloudHtml).not.toContain('Local')
  expect(singleCloudHtml).not.toContain('aria-haspopup')
  expect(singleCloudHtml).not.toContain('Add Cloud Workspace')

  // Multiple Cloud workspaces with localBackend=false: lists Cloud workspaces only, NEVER Local.
  const multiCloudHtml = renderToStaticMarkup(
    React.createElement(
      loadSwitcher(
        {
          active: true,
          source: 'primary',
          status: 'connected',
          activeWorkspaceId: 'ws_example',
          workspaces: [exampleWorkspace, personalWorkspace],
          url: exampleWorkspace.url,
          user: { id: 'u1', org_id: 'org_example', org_name: 'Example Systems', email: 'alex@example.com', display_name: 'Alex', role: 'admin' },
          error: null,
          oidcLoginUrl: '/v1/auth/login',
          capabilities: { spaces: true },
          lastSyncAt: '',
        },
        { localBackend: false, forcePopoverOpen: true },
      ),
    ),
  )
  expect(multiCloudHtml).toContain('Example Systems')
  expect(multiCloudHtml).toContain('Personal Cloud')
  expect(multiCloudHtml).not.toContain('Local')
  expect(multiCloudHtml).not.toContain('Add Cloud Workspace')
})

test('SidebarWorkspaceSwitcher delegates workspace switching without duplicate navigation', async () => {
  const switched = []
  const navigated = []
  const Switcher = loadSwitcher(
    {
      active: true,
      source: 'attached',
      status: 'connected',
      activeWorkspaceId: 'ws_example',
      workspaces: [exampleWorkspace, personalWorkspace],
      url: exampleWorkspace.url,
      user: { id: 'u1', org_id: 'org_example', org_name: 'Example Systems', email: 'alex@example.com', display_name: 'Alex', role: 'admin' },
      error: null,
      oidcLoginUrl: '/v1/auth/login',
      capabilities: { spaces: true },
      lastSyncAt: '',
    },
    {
      localBackend: true,
      forcePopoverOpen: true,
      pathname: '/sessions/thr_123',
      onSwitchWorkspace: async (id) => {
        switched.push(id)
        return null
      },
      onNavigate: (target) => navigated.push(target),
    },
  )

  const buttons = workspaceRows(Switcher)

  // buttons[0] = ws_example (active), buttons[1] = ws_personal, buttons[2] = local
  expect(buttons).toHaveLength(3)
  buttons[1].props.onClick()
  await settled()
  expect(switched).toEqual(['ws_personal'])
  buttons[2].props.onClick()
  await settled()
  expect(switched).toEqual(['ws_personal', 'local'])
  // Navigation on workspace switch is owned solely by __root.tsx via onWorkspaceChange.
  expect(navigated).toEqual([])
})

test('the app changes workspace only once the passage has closed over the old one, then the passage arrives or is cancelled', async () => {
  const events = []
  let shut
  const rows = workspaceRows(
    loadSwitcher(
      { ...noWorkspaces, active: true, source: 'attached', status: 'connected', activeWorkspaceId: 'ws_example', workspaces: [exampleWorkspace, personalWorkspace] },
      {
        forcePopoverOpen: true,
        onTurn: (places, from, to) => {
          events.push(['turn', places, from, to])
          const closed = new Promise((resolve) => {
            shut = resolve
          })
          return { shut: closed, arrive: async () => events.push(['arrive']), cancel: () => events.push(['cancel']) }
        },
        onSwitchWorkspace: async (id) => {
          events.push(['switch', id])
          return id === 'local' ? 'Local is not running' : null
        },
      },
    ),
  )
  const places = [
    { id: 'ws_example', name: 'Example Systems' },
    { id: 'ws_personal', name: 'Personal Cloud' },
    { id: 'local', name: 'Local' },
  ]

  rows[1].props.onClick()
  await settled()
  expect(events).toEqual([['turn', places, 0, 1]])
  shut()
  await settled()
  expect(events.slice(1)).toEqual([['switch', 'ws_personal'], ['arrive']])

  events.length = 0
  rows[2].props.onClick()
  shut()
  await settled()
  expect(events).toEqual([['turn', places, 0, 2], ['switch', 'local'], ['cancel']])
})

test('workspaceSwitchNavigation leaves workspace-specific routes cleanly and preserves Settings', async () => {
  const { workspaceSwitchNavigation } = await import('../../lib/workspaceNavigation.ts')
  const cases = [
    { pathname: '/sessions/thr_123', search: {}, expected: { to: '/new' } },
    { pathname: '/spaces/sp_123', search: {}, expected: { to: '/new' } },
    { pathname: '/bots/bot_42', search: {}, expected: { to: '/bots' } },
    { pathname: '/apps/app_9', search: {}, expected: { to: '/apps' } },
    { pathname: '/loops/lp_3', search: {}, expected: { to: '/loops' } },
    { pathname: '/plugins/tenders', search: {}, expected: { to: '/new' } },
    { pathname: '/apps/crm/show_crm', search: {}, expected: { to: '/apps' } },
    { pathname: '/new', search: { space: 'spc_old' }, expected: { to: '/new', search: {}, replace: true } },
    {
      pathname: '/bots/bot_42',
      search: { settings: 'plugins' },
      expected: { to: '/bots', search: { settings: 'plugins' }, replace: true },
    },
    {
      pathname: '/apps/app_9',
      search: { settings: 'cloud' },
      expected: { to: '/apps', search: { settings: 'cloud' }, replace: true },
    },
    { pathname: '/apps', search: {}, expected: null },
    { pathname: '/bots', search: {}, expected: null },
  ]

  for (const tc of cases) {
    expect(workspaceSwitchNavigation({ pathname: tc.pathname, search: tc.search })).toEqual(tc.expected)
  }
})

test('Add Cloud Workspace opens the sign-in modal in place instead of navigating to Settings', () => {
  const cloudState = {
    active: true,
    source: 'attached',
    status: 'connected',
    activeWorkspaceId: 'ws_example',
    workspaces: [exampleWorkspace],
    url: exampleWorkspace.url,
    user: null,
    error: null,
    oidcLoginUrl: '/v1/auth/login',
    capabilities: {},
    lastSyncAt: '',
  }
  const navigated = []
  const addingCalls = []
  const Switcher = loadSwitcher(cloudState, {
    forcePopoverOpen: true,
    onNavigate: (target) => navigated.push(target),
    onSetAdding: (value) => addingCalls.push(value),
  })
  let addRow
  const walk = (node) => {
    if (!node || typeof node !== 'object') return
    if (Array.isArray(node)) return node.forEach(walk)
    if (node.props?.role === 'menuitem') addRow = node
    walk(node.props?.children)
  }
  walk(Switcher({}))
  addRow.props.onClick()
  expect(addingCalls).toEqual([true])
  expect(navigated).toEqual([])

  const openHtml = renderToStaticMarkup(React.createElement(loadSwitcher(cloudState, { adding: true })))
  expect(openHtml).toContain('data-add-cloud-modal="open"')
})

test('SidebarWorkspaceSwitcher shows each org icon: uploaded image, preset, or the initial', () => {
  const html = renderToStaticMarkup(
    React.createElement(
      loadSwitcher(
        {
          ...noWorkspaces,
          active: true,
          source: 'attached',
          status: 'connected',
          activeWorkspaceId: 'ws_example',
          workspaces: [
            { ...exampleWorkspace, org_icon: { kind: 'image', url: '/v1/org/icon?v=abc' } },
            { ...personalWorkspace, org_icon: { kind: 'preset', preset: 'rocket' } },
            { ...personalWorkspace, id: 'ws_other', org_name: 'Other Org', org_icon: { kind: 'preset', preset: 'retired-preset' } },
          ],
        },
        { localBackend: true, forcePopoverOpen: true },
      ),
    ),
  )
  const rows = [...html.matchAll(/<button role="menuitemradio"[^>]*>(.*?)<\/button>/g)].map((match) => match[1])

  expect(rows[0]).toContain('<img src="https://backend.example/v1/org/icon?v=abc&amp;ticket=t"')
  expect(rows[1]).toContain('lucide-rocket')
  expect(rows[2]).not.toContain('<svg')
  expect(rows[2]).toContain('>O</span>')
})

test('in the nav rail the switcher is the workspace mark alone, named by its tooltip, with the same menu', () => {
  const state = {
    active: false,
    source: 'attached',
    status: 'disconnected',
    activeWorkspaceId: 'local',
    workspaces: [exampleWorkspace],
    url: '',
    user: null,
    error: null,
    oidcLoginUrl: '',
    capabilities: {},
    lastSyncAt: '',
  }
  const closed = renderToStaticMarkup(React.createElement(loadSwitcher(state), { rail: true }))
  expect(closed).toContain('aria-label="Workspace: Local"')
  expect(closed).toContain('title="Local"')
  expect(closed.replace(/<[^>]+>/g, '')).toBe('')

  const open = renderToStaticMarkup(React.createElement(loadSwitcher(state, { forcePopoverOpen: true }), { rail: true }))
  expect(open).toContain('Example Systems')
  expect(checkedRow(open)).toBe('Local')
  expect(open).toContain('Add Cloud Workspace')

  expect(renderToStaticMarkup(React.createElement(loadSwitcher(noWorkspaces), { rail: true }))).toBe('')
})
