import { expect, test } from 'bun:test'
import { build } from 'bun'

const bundle = await build({
  entrypoints: [new URL('./appsLibrary.ts', import.meta.url).pathname],
  target: 'browser',
  format: 'cjs',
  define: { 'import.meta.env': '{}' },
})
if (!bundle.success) {
  throw new AggregateError(bundle.logs)
}
const mod = { exports: {} }
new Function('module', 'exports', await bundle.outputs[0].text())(mod, mod.exports)
const { libraryApps } = mod.exports

const app = (id, fields = {}) => ({ id, title: id, kind: 'artifact', visibility: 'private', ...fields })
const published = [
  app('mine'),
  app('document', { kind: 'document', visibility: 'team' }),
  app('theirs', { is_owner: false, visibility: 'selected' }),
  app('board', { kind: 'board' }),
  app('shared-board', { kind: 'board', is_owner: false, visibility: 'team' }),
]
const installed = [
  { appId: 'document', title: 'Stale title', kind: 'document' },
  { appId: 'gone', title: 'Gone', kind: 'artifact', installedVersion: 3 },
]
const ids = (tab) => libraryApps(tab, published, installed).map((item) => item.id)

test('boards are listed on the Boards tab and on no other', () => {
  expect(ids('boards')).toEqual(['board', 'shared-board'])
  expect(ids('mine')).toEqual(['mine', 'document'])
  expect(ids('team')).toEqual(['document', 'theirs'])
  expect(ids('connected')).toEqual([])
})

test('Installed lists the published app, or the install record once the app is gone', () => {
  const apps = libraryApps('desktop', published, installed)
  expect(apps.map((item) => [item.id, item.title])).toEqual([['document', 'document'], ['gone', 'Gone']])
  expect(apps[1]).toMatchObject({ kind: 'artifact', current_version: 3, visibility: 'private' })
})
