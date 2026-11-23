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
    '@/components/ui/SearchField',
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
          useState: (initial) => [typeof initial === 'function' ? initial() : initial, () => {}],
        }
      }
      if (name === '@/components/sidebar/SidebarWorkspaceSwitcher') {
        return {
          SidebarWorkspaceSwitcher: ({ className }) =>
            cloudState.workspaces.length > 0
              ? React.createElement('div', { 'data-switcher': 'true', className }, 'Switcher')
              : null,
        }
      }
      if (name === '@/components/ui/SearchField') {
        return {
          SearchField: ({ placeholder }) => React.createElement('input', { placeholder }),
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
      onAddWorkspace: () => {},
      onClose: () => {},
    }),
  )
}

const casWorkspace = {
  id: 'ws_cas',
  url: 'https://cas.jaz.example',
  org_id: 'org_cas',
  org_name: 'Cambridge Advanced Systems',
  email: 'eng@cas.example',
  display_name: 'Engineer',
  role: 'admin',
}

test('SETTINGS_SECTIONS places Workspace group first and App group second with expected ordering', () => {
  const workspaceIds = SETTINGS_SECTIONS.filter((item) => item.group === 'workspace').map((item) => item.id)
  const appIds = SETTINGS_SECTIONS.filter((item) => item.group === 'app').map((item) => item.id)

  expect(workspaceIds).toEqual([
    'cloud',
    'personalization',
    'memory',
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
    'agents',
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
  expect(localOnlyIds).toEqual(expect.arrayContaining(['cloud', 'agents', 'providers', 'browser', 'computer']))

  const desktopCloudIds = visibleSettingsSections(true, true, true).map((item) => item.id)
  expect(desktopCloudIds).toContain('machines')
  expect(desktopCloudIds).toEqual(expect.arrayContaining(['agents', 'providers', 'browser', 'computer']))

  const cloudOnlyIds = visibleSettingsSections(true, true, false).map((item) => item.id)
  expect(cloudOnlyIds).toContain('cloud')
  expect(cloudOnlyIds).toContain('machines')
  expect(cloudOnlyIds).toEqual(expect.arrayContaining(['general', 'appearance', 'notifications', 'keyboard', 'voice']))
  expect(cloudOnlyIds).not.toContain('agents')
  expect(cloudOnlyIds).not.toContain('providers')
  expect(cloudOnlyIds).not.toContain('browser')
  expect(cloudOnlyIds).not.toContain('computer')

  const cloudOnlyDisconnectedIds = visibleSettingsSections(true, false, false).map((item) => item.id)
  expect(cloudOnlyDisconnectedIds).not.toContain('machines')
  expect(cloudOnlyDisconnectedIds).not.toContain('agents')
  expect(cloudOnlyDisconnectedIds).not.toContain('computer')
})

test('SettingsNav displays active workspace heading, workspace switcher, and App group cleanly across modes', () => {
  // Desktop with active Cloud workspace
  const cloudDesktopHtml = renderSettingsNav({
    cloudState: {
      active: true,
      activeWorkspaceId: 'ws_cas',
      workspaces: [casWorkspace],
    },
    cloudOnly: false,
  })
  expect(cloudDesktopHtml).toContain('data-switcher="true"')
  expect(cloudDesktopHtml).toContain('Cambridge Advanced Systems')
  expect(cloudDesktopHtml).toContain('Workspace')
  expect(cloudDesktopHtml).toContain('Connected Machines')
  expect(cloudDesktopHtml).toContain('App')
  expect(cloudDesktopHtml).toContain('Agents (ACP)')
  expect(cloudDesktopHtml).toContain('Computer Use')
  expect(cloudDesktopHtml.indexOf('Cambridge Advanced Systems')).toBeLessThan(cloudDesktopHtml.indexOf('>App<'))

  // Desktop with Cloud connected, currently switched to Local
  const localWithCloudHtml = renderSettingsNav({
    cloudState: {
      active: false,
      activeWorkspaceId: 'local',
      workspaces: [casWorkspace],
    },
    cloudOnly: false,
  })
  expect(localWithCloudHtml).toContain('data-switcher="true"')
  expect(localWithCloudHtml).toContain('>Local<')
  expect(localWithCloudHtml).not.toContain('Connected Machines')
  expect(localWithCloudHtml).toContain('Agents (ACP)')

  // Pure Local desktop (0 Cloud workspaces): no switcher chrome, clean Workspace + App groups
  const pureLocalHtml = renderSettingsNav({
    cloudState: {
      active: false,
      activeWorkspaceId: 'local',
      workspaces: [],
    },
    cloudOnly: false,
  })
  expect(pureLocalHtml).not.toContain('data-switcher="true"')
  expect(pureLocalHtml).toContain('>Workspace<')
  expect(pureLocalHtml).toContain('>App<')

  // Cloud-only mode: hides device-only sections and never renders Local
  const cloudOnlyHtml = renderSettingsNav({
    cloudState: {
      active: true,
      activeWorkspaceId: 'ws_cas',
      workspaces: [casWorkspace],
    },
    cloudOnly: true,
  })
  expect(cloudOnlyHtml).toContain('Cambridge Advanced Systems')
  expect(cloudOnlyHtml).toContain('Connected Machines')
  expect(cloudOnlyHtml).not.toContain('Agents (ACP)')
  expect(cloudOnlyHtml).not.toContain('Model Providers')
  expect(cloudOnlyHtml).not.toContain('Browser')
  expect(cloudOnlyHtml).not.toContain('Computer Use')
  expect(cloudOnlyHtml).not.toContain('Local')
})
