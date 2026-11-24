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
    '@/components/ui/Popover',
    '@/components/ui/toast',
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

const casWorkspace = {
  id: 'ws_cas',
  url: 'https://cas.jaz.example',
  org_id: 'org_cas',
  org_name: 'Cambridge Advanced Systems',
  email: 'august@cas.example',
  display_name: 'August',
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
          activeWorkspaceId: 'ws_cas',
          workspaces: [casWorkspace, personalWorkspace],
          url: casWorkspace.url,
          user: { id: 'u1', org_id: 'org_cas', org_name: 'Cambridge Advanced Systems', email: 'august@cas.example', display_name: 'August', role: 'admin' },
          error: null,
          oidcLoginUrl: '/v1/auth/login',
          capabilities: { spaces: true },
          lastSyncAt: '',
        },
        { localBackend: true, forcePopoverOpen: true },
      ),
    ),
  )

  expect(cloudActiveHtml).toContain('aria-label="Workspace: Cambridge Advanced Systems"')
  expect(cloudActiveHtml).toContain('Cambridge Advanced Systems')
  expect(cloudActiveHtml).toContain('Personal Cloud')
  expect(cloudActiveHtml).toContain('Local')
  expect(cloudActiveHtml).toContain('Add Cloud Workspace')
  expect(cloudActiveHtml).not.toContain('august@cas.example')
  expect(cloudActiveHtml).not.toContain('cas.jaz.example')
  expect(cloudActiveHtml).not.toContain('This computer')
  expect(checkedRow(cloudActiveHtml)).toBe('CCambridge Advanced Systems')

  const localActiveHtml = renderToStaticMarkup(
    React.createElement(
      loadSwitcher(
        {
          active: false,
          source: 'attached',
          status: 'disconnected',
          activeWorkspaceId: 'local',
          workspaces: [casWorkspace],
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
  expect(localActiveHtml).toContain('Cambridge Advanced Systems')
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
          activeWorkspaceId: 'ws_cas',
          workspaces: [casWorkspace],
          url: casWorkspace.url,
          user: { id: 'u1', org_id: 'org_cas', org_name: 'Cambridge Advanced Systems', email: 'august@cas.example', display_name: 'August', role: 'admin' },
          error: null,
          oidcLoginUrl: '/v1/auth/login',
          capabilities: { spaces: true },
          lastSyncAt: '',
        },
        { localBackend: false, forcePopoverOpen: true },
      ),
    ),
  )
  expect(singleCloudHtml).toContain('Cambridge Advanced Systems')
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
          activeWorkspaceId: 'ws_cas',
          workspaces: [casWorkspace, personalWorkspace],
          url: casWorkspace.url,
          user: { id: 'u1', org_id: 'org_cas', org_name: 'Cambridge Advanced Systems', email: 'august@cas.example', display_name: 'August', role: 'admin' },
          error: null,
          oidcLoginUrl: '/v1/auth/login',
          capabilities: { spaces: true },
          lastSyncAt: '',
        },
        { localBackend: false, forcePopoverOpen: true },
      ),
    ),
  )
  expect(multiCloudHtml).toContain('Cambridge Advanced Systems')
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
      activeWorkspaceId: 'ws_cas',
      workspaces: [casWorkspace, personalWorkspace],
      url: casWorkspace.url,
      user: { id: 'u1', org_id: 'org_cas', org_name: 'Cambridge Advanced Systems', email: 'august@cas.example', display_name: 'August', role: 'admin' },
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

  const tree = Switcher({})
  const buttons = []
  const walk = (node) => {
    if (!node || typeof node !== 'object') return
    if (Array.isArray(node)) {
      for (const child of node) walk(child)
      return
    }
    if (node.props?.role === 'menuitemradio') {
      buttons.push(node)
    }
    if (node.props?.trigger) walk(node.props.trigger)
    if (node.props?.children) walk(node.props.children)
  }
  walk(tree)

  // buttons[0] = ws_cas (active), buttons[1] = ws_personal, buttons[2] = local
  expect(buttons).toHaveLength(3)
  await buttons[1].props.onClick()
  expect(switched).toEqual(['ws_personal'])
  await buttons[2].props.onClick()
  expect(switched).toEqual(['ws_personal', 'local'])
  // Navigation on workspace switch is owned solely by __root.tsx via onWorkspaceChange.
  expect(navigated).toEqual([])
})

test('workspaceSwitchNavigation leaves workspace-specific routes cleanly and preserves Settings', async () => {
  const { workspaceSwitchNavigation } = await import('../../lib/workspaceNavigation.ts')
  const cases = [
    { pathname: '/sessions/thr_123', search: {}, expected: { to: '/new' } },
    { pathname: '/spaces/sp_123', search: {}, expected: { to: '/new' } },
    { pathname: '/bots/bot_42', search: {}, expected: { to: '/bots' } },
    { pathname: '/boards/brd_9', search: {}, expected: { to: '/boards' } },
    { pathname: '/loops/lp_3', search: {}, expected: { to: '/loops' } },
    { pathname: '/plugins/tenders', search: {}, expected: { to: '/new' } },
    { pathname: '/apps/crm/show_crm', search: {}, expected: { to: '/new' } },
    { pathname: '/new', search: { space: 'spc_old' }, expected: { to: '/new', search: {}, replace: true } },
    {
      pathname: '/bots/bot_42',
      search: { settings: 'plugins' },
      expected: { to: '/bots', search: { settings: 'plugins' }, replace: true },
    },
    {
      pathname: '/boards/brd_9',
      search: { settings: 'cloud' },
      expected: { to: '/boards', search: { settings: 'cloud' }, replace: true },
    },
    { pathname: '/boards', search: {}, expected: null },
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
    activeWorkspaceId: 'ws_cas',
    workspaces: [casWorkspace],
    url: casWorkspace.url,
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
