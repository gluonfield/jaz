export const BROWSER_PROFILE_CHANNELS = {
  dismissed: 'jaz:browser-profile-dismissed',
  dismiss: 'jaz:browser-profile-dismiss',
  list: 'jaz:browser-profile-list',
  sites: 'jaz:browser-profile-sites',
  import: 'jaz:browser-profile-import',
}

export type BrowserProfile = {
  id: string
  browser: string
  name: string
}

export type BrowserCookieSite = {
  domain: string
  cookies: number
}

export type BrowserImportResult = {
  imported: number
  failed: number
}

export interface BrowserProfileAPI {
  dismissed(): Promise<boolean>
  dismiss(): Promise<void>
  list(): Promise<BrowserProfile[]>
  sites(profileId: string): Promise<BrowserCookieSite[]>
  import(profileId: string, domains: string[]): Promise<BrowserImportResult>
}
