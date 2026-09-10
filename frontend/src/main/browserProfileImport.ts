import { access, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { app, ipcMain, session, type IpcMainInvokeEvent } from 'electron'
import { BROWSER_PROFILE_CHANNELS, type BrowserProfileAPI } from '@shared/browserProfile'
import { PREVIEW_PARTITION } from '@shared/preview'
import { discoverBrowserProfiles, type LocalBrowserProfile } from '@main/browserProfiles'
import { importProfileCookies, listCookieSites } from '@main/browserCookies'
import { isTrustedRendererURL } from '@main/permissions'

export class BrowserProfileImporter implements BrowserProfileAPI {
  private importing = false

  constructor(
    private readonly discover: () => Promise<LocalBrowserProfile[]> = discoverBrowserProfiles,
    private readonly password?: (service: string) => Promise<string>,
  ) {}

  async dismissed(): Promise<boolean> {
    try {
      await access(this.marker())
      return true
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code === 'ENOENT') {
        return false
      }
      throw error
    }
  }

  async dismiss(): Promise<void> {
    await writeFile(this.marker(), '', { mode: 0o600 })
  }

  async list() {
    return (await this.discover()).map(({ id, browser, name }) => ({ id, browser, name }))
  }

  async sites(profileId: string) {
    return listCookieSites(await this.profile(profileId))
  }

  async import(profileId: string, domains: string[]) {
    if (this.importing) {
      throw new Error('A browser import is already running.')
    }
    this.importing = true
    try {
      const profile = await this.profile(profileId)
      const cookies = session.fromPartition(PREVIEW_PARTITION).cookies
      const result = await importProfileCookies(profile, domains, (cookie) => cookies.set(cookie), this.password)
      await cookies.flushStore()
      if (result.imported > 0) {
        await this.dismiss()
      }
      return result
    } finally {
      this.importing = false
    }
  }

  private async profile(id: string): Promise<LocalBrowserProfile> {
    const profile = (await this.discover()).find((profile) => profile.id === id)
    if (!profile) {
      throw new Error('This browser profile is no longer available. Choose another profile.')
    }
    return profile
  }

  private marker(): string {
    return join(app.getPath('userData'), 'browser-profile-import-dismissed')
  }
}

export function installBrowserProfileImport(importer = new BrowserProfileImporter()): void {
  ipcMain.handle(BROWSER_PROFILE_CHANNELS.dismissed, (event) => fromApp(event, () => importer.dismissed()))
  ipcMain.handle(BROWSER_PROFILE_CHANNELS.dismiss, (event) => fromApp(event, () => importer.dismiss()))
  ipcMain.handle(BROWSER_PROFILE_CHANNELS.list, (event) => fromApp(event, () => importer.list()))
  ipcMain.handle(BROWSER_PROFILE_CHANNELS.sites, (event, profileId: string) => fromApp(event, () => importer.sites(profileId)))
  ipcMain.handle(BROWSER_PROFILE_CHANNELS.import, (event, profileId: string, domains: string[]) => fromApp(event, () => importer.import(profileId, domains)))
}

function fromApp(event: IpcMainInvokeEvent, action: () => unknown): unknown {
  if (event.sender.getType() !== 'window' || event.senderFrame !== event.sender.mainFrame || !isTrustedRendererURL(event.senderFrame.url)) {
    throw new Error('Browser imports must be started from Jaz.')
  }
  return action()
}
