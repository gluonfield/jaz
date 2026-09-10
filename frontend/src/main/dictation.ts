import { execFile } from 'node:child_process'
import { existsSync } from 'node:fs'
import { join } from 'node:path'
import { promisify } from 'node:util'
import { app, ipcMain, type IpcMainInvokeEvent, type WebContents } from 'electron'
import type { DictationAvailability } from '../shared/dictation'
import { startDictationProcess } from './dictationProcess'
import { canGrantAppPermission } from './permissions'

const execute = promisify(execFile)

export function registerDictation(): void {
  const executable = app.isPackaged
    ? join(process.resourcesPath, 'bin/jaz-dictation')
    : join(app.getAppPath(), 'resources/bin/jaz-dictation')
  let active: {
    id: string
    owner: WebContents
    process: ReturnType<typeof startDictationProcess>
  } | undefined

  const availability = async (): Promise<DictationAvailability> => {
    if (process.platform !== 'darwin') {
      return { available: false, reason: 'Native dictation is currently available on macOS 26 or later.' }
    }
    if (!existsSync(executable)) {
      return { available: false, reason: 'Rebuild or update Jaz to install native dictation.' }
    }
    try {
      const { stdout } = await execute(executable, ['--check', '--locale', app.getLocale()], { timeout: 15_000 })
      const result = JSON.parse(stdout)
      return result.available === true
        ? { available: true }
        : { available: false, reason: typeof result.reason === 'string' ? result.reason : 'Native dictation is unavailable.' }
    } catch {
      return { available: false, reason: 'Native dictation could not be initialized.' }
    }
  }
  const validate = (event: IpcMainInvokeEvent, id: unknown): string => {
    if (!canGrantAppPermission(event.sender, 'media', {
      isMainFrame: event.senderFrame === event.sender.mainFrame,
      requestingUrl: event.senderFrame?.url,
    })) {
      throw new Error('Dictation is only available in Jaz windows.')
    }
    if (typeof id !== 'string' || !/^[a-zA-Z0-9-]{1,64}$/.test(id)) {
      throw new Error('Invalid dictation session.')
    }
    return id
  }
  ipcMain.handle('jaz:dictation:availability', availability)
  ipcMain.handle('jaz:dictation:start', (event, value) => {
    const id = validate(event, value)
    if (active) {
      throw new Error('Dictation is already recording in another composer.')
    }
    if (process.platform !== 'darwin' || !existsSync(executable)) {
      throw new Error('Native dictation is unavailable.')
    }
    const owner = event.sender
    const cancel = () => active?.owner === owner && active.process.cancel()
    const navigate = (_event: unknown, _url: string, inPlace: boolean, isMainFrame: boolean) => {
      if (isMainFrame && !inPlace) {
        cancel()
      }
    }
    owner.once('destroyed', cancel)
    owner.once('render-process-gone', cancel)
    owner.on('did-start-navigation', navigate)
    active = {
      id,
      owner,
      process: startDictationProcess(executable, ['--locale', app.getLocale()], (update) => {
        if (!owner.isDestroyed()) {
          owner.send('jaz:dictation:event', id, update)
        }
      }, () => {
        owner.removeListener('destroyed', cancel)
        owner.removeListener('render-process-gone', cancel)
        owner.removeListener('did-start-navigation', navigate)
        active = undefined
      }),
    }
  })
  for (const action of ['stop', 'cancel'] as const) {
    ipcMain.handle('jaz:dictation:' + action, (event, value) => {
      const id = validate(event, value)
      if (active?.owner === event.sender && active.id === id) {
        active.process[action]()
      }
    })
  }
  app.on('before-quit', () => active?.process.cancel())
}
