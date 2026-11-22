import { afterEach, expect, test } from 'bun:test'
import { deriveGrantId, loadLocalMachineConfig, saveLocalMachineConfig } from './cloudMachineConfig'

const originalStorage = globalThis.localStorage

afterEach(() => {
  if (originalStorage) {
    globalThis.localStorage = originalStorage
  } else {
    delete globalThis.localStorage
  }
})

function mockStorage() {
  const map = new Map()
  globalThis.localStorage = {
    getItem: (key) => map.get(key) ?? null,
    setItem: (key, value) => map.set(key, value),
    removeItem: (key) => map.delete(key),
  }
  return map
}

test('loadLocalMachineConfig and saveLocalMachineConfig round-trip grants and opt-in exec flag', () => {
  mockStorage()
  const initial = loadLocalMachineConfig()
  expect(initial.allowExec).toBe(false)
  expect(initial.grants).toEqual([])
  expect(initial.deviceId).toMatch(/^desktop-/)

  saveLocalMachineConfig({
    deviceId: 'mac-studio',
    displayName: 'Studio',
    allowExec: true,
    grants: [{ id: 'work', root_path: '/Users/dev/work', mode: 'rw' }],
  })
  expect(loadLocalMachineConfig()).toEqual({
    deviceId: 'mac-studio',
    displayName: 'Studio',
    allowExec: true,
    grants: [{ id: 'work', root_path: '/Users/dev/work', mode: 'rw' }],
  })
})

test('loadLocalMachineConfig filters invalid grant modes and handles corrupt JSON', () => {
  const map = mockStorage()
  map.set('jaz.cloud.machineConfig', '{broken')
  expect(loadLocalMachineConfig().grants).toEqual([])

  map.set('jaz.cloud.machineConfig', JSON.stringify({
    deviceId: 'win-1',
    allowExec: 'yes',
    grants: [
      { id: 'ok', root_path: 'C:\\Work', mode: 'rwd' },
      { id: 'bad', root_path: 'C:\\Nope', mode: 'admin' },
    ],
  }))
  const loaded = loadLocalMachineConfig()
  expect(loaded.allowExec).toBe(false)
  expect(loaded.grants).toEqual([{ id: 'ok', root_path: 'C:\\Work', mode: 'rwd' }])
})

test('deriveGrantId creates clean unique slugs from Unix and Windows folder paths', () => {
  const first = deriveGrantId('/Users/dev/My Repo/', [])
  expect(first).toBe('my-repo')
  const second = deriveGrantId('C:\\Projects\\My Repo', [{ id: 'my-repo', root_path: '/a', mode: 'ro' }])
  expect(second).toBe('my-repo-2')
})
