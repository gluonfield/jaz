import { expect, test } from 'bun:test'
import { build } from 'bun'
import { createRequire } from 'node:module'
import * as React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { SETTINGS_SECTIONS, visibleSettingsSections } from './sections'

const navBundle = await build({
  entrypoints: [new URL('./SettingsNav.tsx', import.meta.url).pathname],
  target: 'browser',
  format: 'cjs',
  external: [
    'react',
    'react/jsx-runtime',
    'react/jsx-dev-runtime',
    'lucide-react',
    '@/components/sidebar/SidebarWorkspaceSwitcher',
    '@/lib/cloudSession',
  ],
  define: { 'import.meta.env': '{}' },
})
if (!navBundle.success) {
  throw new AggregateError(navBundle.logs)
}
const evaluateNav = new Function('require', 'module', 'exports', await navBundle.outputs[0].text())
const require = createRequire(import.meta.url)

function renderSettingsNav({ cloudState, cloudOnly = false, section = 'cloud' }) {
  const mod = { exports: {} }
  evaluateNav(
    (name) => {
      if (name === 'react') {
        return {
          ...React,
          useSyncExternalStore: (_sub, snap) => snap(),
        }
      }
      if (name === '@/components/sidebar/SidebarWorkspaceSwitcher') {
        return {
          SidebarWorkspaceSwitcher: ({ className }) =>
            React.createElement('div', { 'data-switcher': 'true', className }, 'Switcher'),
        }
      }
      if (name === '@/lib/cloudSession') {
        return {
          useCloudSession: () => cloudState,
          useIsCloudOnly: () => cloudOnly,
        }
      }
      return require(name)
    },
    mod,
    mod.exports,
  )
  return renderToStaticMarkup(
    React.createElement(mod.exports.SettingsNav, {
      section,
      onSectionChange: () => {},
      onClose: () => {},
    }),
  )
}

const exampleWorkspace = {
  id: 'ws_example',
  url: 'https://workspaces.example.com',
  org_id: 'org_example',
  org_name: 'Example Systems',
  email: 'eng@example.com',
  display_name: 'Engineer',
  role: 'admin',
}

test('SETTINGS_SECTIONS places Workspace group first and App group second with expected ordering', () => {
  const workspaceIds = SETTINGS_SECTIONS.filter((item) => item.group === 'workspace').map((item) => item.id)
  const appIds = SETTINGS_SECTIONS.filter((item) => item.group === 'app').map((item) => item.id)

  expect(workspaceIds).toEqual([
    'cloud',
    'admin',
    'personalization',
    'memory',
    'vault',
    'agents',
    'plugins',
    'mcp',
    'machines',
    'usage',
    'archived',
  ])
  expect(appIds).toEqual([
    'general',
    'appearance',
    'notifications',
    'keyboard',
    'voice',
    'providers',
    'browser',
    'computer',
  ])
  expect(SETTINGS_SECTIONS.map((item) => item.id)).toEqual([...workspaceIds, ...appIds])
  expect(SETTINGS_SECTIONS[0]).toMatchObject({ id: 'cloud', label: 'Workspace', group: 'workspace' })
})

test('visibleSettingsSections enforces requiresCloud and requiresLocal boundaries', () => {
  const localOnlyIds = visibleSettingsSections(true, false, true).map((item) => item.id)
  expect(localOnlyIds).not.toContain('machines')
  expect(localOnlyIds).not.toContain('github')
  expect(localOnlyIds).toEqual(expect.arrayContaining(['cloud', 'vault', 'agents', 'providers', 'browser', 'computer']))

  const desktopCloudIds = visibleSettingsSections(true, true, true).map((item) => item.id)
  expect(desktopCloudIds).toContain('machines')
  expect(desktopCloudIds).toContain('vault')
  expect(desktopCloudIds).not.toContain('github')
  expect(desktopCloudIds).not.toContain('memory')
  expect(desktopCloudIds).toEqual(expect.arrayContaining(['agents', 'providers', 'browser', 'computer']))

  const cloudOnlyIds = visibleSettingsSections(true, true, false).map((item) => item.id)
  expect(cloudOnlyIds).toContain('cloud')
  expect(cloudOnlyIds).toContain('machines')
  expect(cloudOnlyIds).toContain('vault')
  expect(cloudOnlyIds).not.toContain('github')
  expect(cloudOnlyIds).toContain('agents')
  expect(cloudOnlyIds).toEqual(expect.arrayContaining(['general', 'appearance', 'notifications', 'keyboard']))
  expect(cloudOnlyIds).not.toContain('memory')
  expect(cloudOnlyIds).not.toContain('voice')
  expect(cloudOnlyIds).not.toContain('providers')
  expect(cloudOnlyIds).not.toContain('browser')
  expect(cloudOnlyIds).not.toContain('computer')

  const cloudOnlyDisconnectedIds = visibleSettingsSections(true, false, false).map((item) => item.id)
  expect(cloudOnlyDisconnectedIds).not.toContain('machines')
  expect(cloudOnlyDisconnectedIds).not.toContain('vault')
  expect(cloudOnlyDisconnectedIds).not.toContain('github')
  expect(cloudOnlyDisconnectedIds).not.toContain('agents')
  expect(cloudOnlyDisconnectedIds).not.toContain('computer')
})

test('SettingsNav heads workspace settings with the switcher alone and keeps App settings separate', () => {
  const cloudDesktopHtml = renderSettingsNav({
    cloudState: {
      active: true,
      activeWorkspaceId: 'ws_example',
      workspaces: [exampleWorkspace],
    },
    cloudOnly: false,
  })
  const switcherAt = cloudDesktopHtml.indexOf('data-switcher="true"')
  const appAt = cloudDesktopHtml.indexOf('>App<')
  expect(switcherAt).toBeGreaterThan(-1)
  expect(switcherAt).toBeLessThan(cloudDesktopHtml.indexOf('>Workspace<'))
  expect(cloudDesktopHtml.indexOf('Connected Machines')).toBeLessThan(appAt)
  expect(cloudDesktopHtml).not.toContain('GitHub')
  expect(cloudDesktopHtml.indexOf('Agents (ACP)')).toBeLessThan(appAt)
  expect(cloudDesktopHtml).toContain('Computer Use')
  expect(cloudDesktopHtml).not.toContain('Example Systems')
  expect(cloudDesktopHtml).not.toContain('<input')

  const localWithCloudHtml = renderSettingsNav({
    cloudState: {
      active: false,
      activeWorkspaceId: 'local',
      workspaces: [exampleWorkspace],
    },
    cloudOnly: false,
  })
  expect(localWithCloudHtml).toContain('data-switcher="true"')
  expect(localWithCloudHtml).not.toContain('>Local<')
  expect(localWithCloudHtml).not.toContain('Connected Machines')
  expect(localWithCloudHtml).not.toContain('GitHub')
  expect(localWithCloudHtml).toContain('Agents (ACP)')

  // Even if cloudState.active is stale while activeWorkspaceId is local, Cloud-only sections stay hidden.
  const staleLocalHtml = renderSettingsNav({
    cloudState: {
      active: true,
      activeWorkspaceId: 'local',
      workspaces: [exampleWorkspace],
    },
    cloudOnly: false,
  })
  expect(staleLocalHtml).not.toContain('Connected Machines')
  expect(staleLocalHtml).not.toContain('GitHub')

  const pureLocalHtml = renderSettingsNav({
    cloudState: {
      active: false,
      activeWorkspaceId: 'local',
      workspaces: [],
    },
    cloudOnly: false,
  })
  expect(pureLocalHtml).toContain('data-switcher="true"')
  expect(pureLocalHtml).toContain('>Workspace<')
  expect(pureLocalHtml).toContain('>App<')
  expect(pureLocalHtml).not.toContain('GitHub')

  const cloudOnlyHtml = renderSettingsNav({
    cloudState: {
      active: true,
      activeWorkspaceId: 'ws_example',
      workspaces: [exampleWorkspace],
    },
    cloudOnly: true,
  })
  expect(cloudOnlyHtml).toContain('data-switcher="true"')
  expect(cloudOnlyHtml).toContain('Connected Machines')
  expect(cloudOnlyHtml).not.toContain('GitHub')
  expect(cloudOnlyHtml).toContain('Agents (ACP)')
  expect(cloudOnlyHtml).not.toContain('Model Providers')
  expect(cloudOnlyHtml).not.toContain('Browser')
  expect(cloudOnlyHtml).not.toContain('Computer Use')
})

test('Admin is offered only to org admins of an active Cloud workspace', () => {
  const ids = (...args) => visibleSettingsSections(...args).map((item) => item.id)

  expect(ids(true, true, true, true)).toContain('admin')
  expect(ids(true, true, false, true)).toContain('admin')
  expect(ids(true, true, true, false)).not.toContain('admin')

  const cloud = { active: true, activeWorkspaceId: exampleWorkspace.id, workspaces: [exampleWorkspace] }
  const adminNav = renderSettingsNav({ cloudState: { ...cloud, user: { role: 'admin' } } })
  const memberNav = renderSettingsNav({ cloudState: { ...cloud, user: { role: 'member' } } })
  expect(adminNav).toContain('>Admin</span>')
  expect(memberNav).not.toContain('>Admin</span>')
})
