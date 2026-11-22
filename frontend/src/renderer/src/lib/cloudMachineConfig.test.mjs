import { afterEach, expect, test } from 'bun:test'
import {
  buildConnectCloudCommand,
  deriveGrantId,
  loadLocalMachineConfig,
  saveLocalMachineConfig,
  validateGrantId,
  validateGrantPath,
} from './cloudMachineConfig'

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

test('loadLocalMachineConfig and saveLocalMachineConfig scope grants per cloud origin', () => {
  mockStorage()
  const cloudA = 'https://cloud-a.example'
  const cloudB = 'https://cloud-b.example'
  const initial = loadLocalMachineConfig(cloudA)
  expect(initial.allowExec).toBe(false)
  expect(initial.grants).toEqual([])
  expect(initial.deviceId).toMatch(/^desktop-/)

  saveLocalMachineConfig(cloudA, {
    deviceId: 'mac-studio',
    displayName: 'Studio',
    allowExec: true,
    grants: [{ id: 'work', root_path: '/Users/dev/work', mode: 'rw' }],
  })
  expect(loadLocalMachineConfig(cloudA)).toEqual({
    deviceId: 'mac-studio',
    displayName: 'Studio',
    allowExec: true,
    grants: [{ id: 'work', root_path: '/Users/dev/work', mode: 'rw' }],
  })
  expect(loadLocalMachineConfig(cloudB).grants).toEqual([])
})

test('loadLocalMachineConfig filters invalid grant modes, control chars, and corrupt JSON', () => {
  const map = mockStorage()
  const cloud = 'https://cloud.example'
  const key = `jaz.cloud.machineConfig.${cloud}`
  map.set(key, '{broken')
  expect(loadLocalMachineConfig(cloud).grants).toEqual([])

  map.set(key, JSON.stringify({
    deviceId: 'win-1',
    displayName: 'Bad\nName',
    allowExec: 'yes',
    grants: [
      { id: 'ok', root_path: 'C:\\Work', mode: 'rwd' },
      { id: 'bad', root_path: 'C:\\Nope', mode: 'admin' },
      { id: 'eq=bad', root_path: 'C:\\Eq', mode: 'ro' },
      { id: 'nl', root_path: '/tmp/bad\npath', mode: 'ro' },
    ],
  }))
  const loaded = loadLocalMachineConfig(cloud)
  expect(loaded.allowExec).toBe(false)
  expect(loaded.displayName).toBe('')
  expect(loaded.grants).toEqual([{ id: 'ok', root_path: 'C:\\Work', mode: 'rwd' }])
})

test('deriveGrantId creates clean unique slugs from Unix and Windows folder paths', () => {
  const first = deriveGrantId('/Users/dev/My Repo/', [])
  expect(first).toBe('my-repo')
  const second = deriveGrantId('C:\\Projects\\My Repo', [{ id: 'my-repo', root_path: '/a', mode: 'ro' }])
  expect(second).toBe('my-repo-2')
})

test('buildConnectCloudCommand omits --token, supports POSIX and PowerShell quoting, and rejects control characters', () => {
  const posix = buildConnectCloudCommand({
    cloudUrl: 'https://cloud.jaz.example/',
    deviceId: 'mac-1',
    displayName: "Dev's Mac",
    grants: [{ id: 'proj', root_path: "/Users/dev/It's Repo", mode: 'rw' }],
    allowExec: true,
    shell: 'posix',
  })
  expect(posix).not.toContain('--token')
  expect(posix).not.toContain('JAZ_CLOUD_TOKEN')
  expect(posix).toContain('--url https://cloud.jaz.example')
  expect(posix).toContain("--name 'Dev'\\''s Mac'")
  expect(posix).toContain("--grant 'proj=rw:/Users/dev/It'\\''s Repo'")
  expect(posix).toContain('--allow-exec')
  expect(posix).toContain(' \\\n  ')

  const ps = buildConnectCloudCommand({
    cloudUrl: 'https://cloud.jaz.example',
    deviceId: 'win-1',
    displayName: "Dev's PC",
    grants: [{ id: 'proj', root_path: "C:\\Users\\Dev's\\Repo", mode: 'ro' }],
    allowExec: false,
    shell: 'powershell',
  })
  expect(ps).not.toContain('--token')
  expect(ps).toContain("--name 'Dev''s PC'")
  expect(ps).toContain("--grant 'proj=ro:C:\\Users\\Dev''s\\Repo'")
  expect(ps).toContain(' `\n  ')
  expect(ps).not.toContain('--allow-exec')

  expect(validateGrantPath('/tmp/bad\npath')).toContain('control')
  expect(validateGrantId('bad:id')).toContain(':')
  expect(() => buildConnectCloudCommand({
    cloudUrl: 'https://cloud.jaz.example',
    deviceId: 'mac-1',
    displayName: 'Ok',
    grants: [{ id: 'proj', root_path: '/tmp/evil\nrm -rf /', mode: 'ro' }],
    allowExec: false,
    shell: 'posix',
  })).toThrow('control')
})
