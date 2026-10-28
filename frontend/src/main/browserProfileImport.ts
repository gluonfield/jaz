import { ipcMain, session, webContents, type IpcMainInvokeEvent } from 'electron'
import { BROWSER_PROFILE_CHANNELS, type BrowserImportCount, type BrowserImportSelection, type BrowserProfileAPI } from '@shared/browserProfile'
import { PREVIEW_PARTITION } from '@shared/preview'
import { PASSWORD_CHANGED_CHANNEL } from '@shared/browserPasswords'
import { discoverBrowserProfiles } from '@main/browserProfiles'
import { importProfileCookies } from '@main/browserCookies'
import { importProfilePasswords } from '@main/browserPasswordImport'
import { getBrowserPasswordStore } from '@main/browserPasswordStore'
import { readKeychainPassword } from '@main/browserSafeStorage'
import { isTrustedRendererURL } from '@main/permissions'

export class BrowserProfileImporter implements BrowserProfileAPI {
  private importing = false

  constructor(
    private readonly discover = discoverBrowserProfiles,
    private readonly password = readKeychainPassword,
    private readonly store = getBrowserPasswordStore(),
  ) {}

  async list() {
    return (await this.discover()).map(({ id, browser, name, database, passwordDatabases }) => ({ id, browser, name, cookies: Boolean(database), passwords: passwordDatabases.length > 0 }))
  }

  async import(profileId: string, selection: BrowserImportSelection) {
    if (!selection || typeof selection.cookies !== 'boolean' || typeof selection.passwords !== 'boolean' || (!selection.cookies && !selection.passwords)) {
      throw new Error('Choose cookies or saved passwords to import.')
    }
    if (this.importing) {
      throw new Error('A browser import is already running.')
    }
    this.importing = true
    try {
      const profile = (await this.discover()).find((profile) => profile.id === profileId)
      if (!profile) {
        throw new Error('This browser profile is no longer available. Choose another profile.')
      }
      let secret: Promise<string> | undefined
      const unlock = (service: string) => secret ??= this.password(service)
      const cookies = session.fromPartition(PREVIEW_PARTITION).cookies
      const [cookieResult, passwordResult] = await Promise.all([
        selection.cookies ? attemptImport(async () => {
          const result = await importProfileCookies(profile, (cookie) => cookies.set(cookie), unlock)
          await cookies.flushStore()
          return result
        }) : undefined,
        selection.passwords ? attemptImport(() => importProfilePasswords(profile, this.store, unlock)) : undefined,
      ])
      if (passwordResult?.imported) {
        for (const browser of webContents.getAllWebContents()) {
          if (browser.getType() === 'webview' && browser.session === session.fromPartition(PREVIEW_PARTITION)) {
            browser.hostWebContents?.send(PASSWORD_CHANGED_CHANNEL, browser.id)
          }
        }
      }
      return { ...(cookieResult && { cookies: cookieResult }), ...(passwordResult && { passwords: passwordResult }) }
    } finally {
      this.importing = false
    }
  }
}

async function attemptImport(action: () => Promise<BrowserImportCount>): Promise<BrowserImportCount> {
  try {
    return await action()
  } catch (error) {
    return { imported: 0, failed: 0, error: error instanceof Error ? error.message : 'Browser data could not be imported.' }
  }
}

export function installBrowserProfileImport(importer = new BrowserProfileImporter()): void {
  ipcMain.handle(BROWSER_PROFILE_CHANNELS.list, (event) => fromApp(event, () => importer.list()))
  ipcMain.handle(BROWSER_PROFILE_CHANNELS.import, (event, profileId: string, selection: BrowserImportSelection) => fromApp(event, () => importer.import(profileId, selection)))
}

function fromApp(event: IpcMainInvokeEvent, action: () => unknown): unknown {
  if (event.sender.getType() !== 'window' || event.senderFrame !== event.sender.mainFrame || !isTrustedRendererURL(event.senderFrame.url)) {
    throw new Error('Browser imports must be started from Jaz.')
  }
  return action()
}
