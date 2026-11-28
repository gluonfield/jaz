import { expect, test } from 'bun:test'
import { build } from 'bun'

async function loadModule(relativePath) {
  const bundle = await build({
    entrypoints: [new URL(relativePath, import.meta.url).pathname],
    target: 'browser',
    format: 'cjs',
    define: { 'import.meta.env': '{}' },
  })
  if (!bundle.success) {
    throw new AggregateError(bundle.logs)
  }
  const mod = { exports: {} }
  new Function('module', 'exports', await bundle.outputs[0].text())(mod, mod.exports)
  return mod.exports
}

const { libraryApps } = await loadModule('./appsLibrary.ts')
const { matchesDesktopAppScope } = await loadModule('../../../shared/desktopApps.ts')

const app = (id, fields = {}) => ({ id, title: id, kind: 'artifact', visibility: 'private', ...fields })
const published = [
  app('mine'),
  app('document', { kind: 'document', visibility: 'team' }),
  app('theirs', { is_owner: false, visibility: 'selected' }),
  app('board', { kind: 'board' }),
  app('shared-board', { kind: 'board', is_owner: false, visibility: 'team' }),
]
const installed = [
  { appId: 'document', workspaceId: 'local', title: 'Stale title', kind: 'document' },
  { appId: 'gone', workspaceId: 'local', title: 'Gone', kind: 'artifact', icon: { kind: 'emoji', value: '⚡' }, installedVersion: 3 },
  { appId: 'dev-local', workspaceId: 'local', origin: 'http://127.0.0.1:5299', title: 'New data centre builds', kind: 'artifact', installedVersion: 1 },
  { appId: 'cloud-org-a', workspaceId: 'org_a', origin: 'https://staging.jaz.example', title: 'Org A App', kind: 'artifact', installedVersion: 2 },
  { appId: 'cloud-org-a-prod', workspaceId: 'org_a', origin: 'https://prod.jaz.example', title: 'Org A Prod App', kind: 'artifact', installedVersion: 1 },
  { appId: 'cloud-org-b', workspaceId: 'org_b', origin: 'https://staging.jaz.example', title: 'Org B App', kind: 'document', installedVersion: 1 },
]
const ids = (tab) => libraryApps(tab, published, installed).map((item) => item.id)

test('boards are listed on the Boards tab and on no other', () => {
  expect(ids('boards')).toEqual(['board', 'shared-board'])
  expect(ids('mine')).toEqual(['mine', 'document'])
  expect(ids('team')).toEqual(['document', 'theirs'])
  expect(ids('connected')).toEqual([])
})

test('Installed lists the published app, or the install record with icon once the app is gone', () => {
  const apps = libraryApps('desktop', published, installed.slice(0, 2))
  expect(apps.map((item) => [item.id, item.title])).toEqual([['document', 'document'], ['gone', 'Gone']])
  expect(apps[1]).toMatchObject({ kind: 'artifact', current_version: 3, visibility: 'private', icon: { kind: 'emoji', value: '⚡' } })
})

test('matchesDesktopAppScope scopes installed desktop apps strictly by workspace and origin', () => {
  const filterBy = (scope) => installed.filter((record) => matchesDesktopAppScope(record, scope)).map((item) => item.appId)

  expect(filterBy({ workspaceId: 'org_a', origin: 'https://staging.jaz.example', isLocalOrigin: false })).toEqual(['cloud-org-a'])
  expect(filterBy({ workspaceId: 'org_a', origin: 'https://prod.jaz.example', isLocalOrigin: false })).toEqual(['cloud-org-a-prod'])
  expect(filterBy({ workspaceId: 'local', origin: 'http://127.0.0.1:5299', isLocalOrigin: true })).toEqual(['document', 'gone', 'dev-local'])
  expect(filterBy({ workspaceId: 'local', origin: 'https://remote.selfhost.example', isLocalOrigin: false })).toEqual([])
})
