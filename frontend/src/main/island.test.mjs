import { expect, mock, test } from 'bun:test'
import { spawnSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { ISLAND_HEIGHT } from '../shared/notifications'

if (process.env.JAZ_ISLAND_TEST_CHILD === '1') {
  Object.defineProperty(process, 'platform', { value: 'darwin' })
  const ready = {
    model: 'Mac16,5',
    screens: [{ id: 1, notch: { x: 918, width: 220, height: 38 } }],
    focus: 'off',
    focus_access: 'authorized',
    previews: 'always',
    notifications: 'authorized',
  }
  let status = ready
  let requested = ready
  let idle = 'active'
  let workArea = { x: 0, y: 38, width: 1512, height: 944 }
  const calls = []
  const windows = []
  mock.module('node:child_process', () => ({
    execFile: (_file, args, _options, callback) => {
      calls.push(args[0])
      const reply = args[0] === 'request-focus' ? requested : status
      if (reply) callback(null, { stdout: JSON.stringify(reply), stderr: '' })
      else callback(new Error('helper missing'))
    },
  }))
  mock.module('electron', () => ({
    app: { isPackaged: true, accessibilitySupportEnabled: false, getAppPath: () => '/Jaz' },
    ipcMain: { on() {} },
    powerMonitor: { getSystemIdleState: () => idle },
    screen: {
      getCursorScreenPoint: () => ({ x: 10, y: 10 }),
      getDisplayNearestPoint: () => ({ id: 1, internal: true, bounds: { x: 0, y: 0, width: 1512, height: 982 }, workArea }),
    },
    BrowserWindow: class {
      constructor(options) {
        this.options = options
        this.shown = 0
        this.focused = 0
        this.webContents = { send() {} }
        windows.push(this)
      }
      isDestroyed() {
        return false
      }
      once() {}
      setAlwaysOnTop() {}
      setVisibleOnAllWorkspaces() {}
      setContentProtection() {}
      setIgnoreMouseEvents() {}
      loadFile() {}
      setBounds(bounds) {
        this.bounds = bounds
      }
      showInactive() {
        this.shown++
      }
      focus() {
        this.focused++
      }
      hide() {}
    },
  }))
  const { createIsland } = await import('./island')
  const { notchAccess, shareFocus } = await import('./notchHelper')
  const alert = { sessionId: 'thread', key: 'done', state: 'done', kind: 'chat', name: '', avatars: [], title: 'Fix the build', body: 'Fixed.', requestId: '', at: 0 }

  test('the island appears inactive beside the notch only with notifications allowed and Focus known off', async () => {
    const island = createIsland(() => {})
    const shows = async (next, setup = () => {}) => {
      status = next
      setup()
      const shown = await island.present(alert, false)
      island.clear()
      idle = 'active'
      workArea = { x: 0, y: 38, width: 1512, height: 944 }
      return shown
    }
    expect(await shows(ready)).toBe(true)
    expect(windows[0].options).toMatchObject({ focusable: false, type: 'panel', roundedCorners: false })
    expect({ shown: windows[0].shown, focused: windows[0].focused, bounds: windows[0].bounds }).toEqual({
      shown: 1, focused: 0, bounds: { x: 778, y: 0, width: 500, height: ISLAND_HEIGHT },
    })
    const fallbacks = {
      'notifications undecided': await shows({ ...ready, notifications: 'not_determined' }),
      'notifications denied': await shows({ ...ready, notifications: 'denied' }),
      'Focus not shared yet': await shows({ ...ready, focus: 'unknown', focus_access: 'not_determined' }),
      'Focus refused': await shows({ ...ready, focus: 'unknown', focus_access: 'denied' }),
      'Focus on': await shows({ ...ready, focus: 'on' }),
      'helper unavailable': await shows(null),
      'Mac idle': await shows(ready, () => { idle = 'idle' }),
      'full-screen space': await shows(ready, () => { workArea = { x: 0, y: 0, width: 1512, height: 982 } }),
    }
    expect(Object.entries(fallbacks).filter(([, shown]) => shown)).toEqual([])
    expect(calls.includes('request-focus')).toBe(false)
  })

  test('macOS is asked to share Focus only when a person asks and it is undecided', async () => {
    const access = async (next) => {
      status = next
      return notchAccess()
    }
    expect(await access(ready)).toBe('ready')
    expect(await access({ ...ready, notifications: 'not_determined' })).toBe('notifications')
    expect(await access({ ...ready, focus: 'unknown', focus_access: 'not_determined' })).toBe('ask')
    expect(await access({ ...ready, focus: 'unknown', focus_access: 'denied' })).toBe('denied')
    expect(await access({ ...ready, focus: 'unknown', focus_access: 'restricted' })).toBe('unavailable')
    expect(await access(null)).toBe('unavailable')
    expect(calls.includes('request-focus')).toBe(false)
    status = { ...ready, focus: 'unknown', focus_access: 'denied' }
    expect(await shareFocus()).toBe('denied')
    expect(calls.includes('request-focus')).toBe(false)
    status = { ...ready, focus: 'unknown', focus_access: 'not_determined' }
    requested = { ...ready, focus: 'unknown', focus_access: 'denied' }
    expect(await shareFocus()).toBe('denied')
    requested = ready
    expect(await shareFocus()).toBe('ready')
    expect(calls.filter((call) => call === 'request-focus')).toHaveLength(2)
  })
} else {
  test('island placement against the helper and Electron boundary in an isolated fixture', () => {
    const result = spawnSync(process.execPath, ['test', fileURLToPath(import.meta.url)], {
      env: { ...process.env, JAZ_ISLAND_TEST_CHILD: '1' },
      encoding: 'utf8',
      timeout: 8000,
    })
    expect({ status: result.status, output: result.status ? result.stderr : '' }).toEqual({ status: 0, output: '' })
  })
}
