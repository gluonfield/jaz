import { createHash } from 'node:crypto'
import { access, readFile } from 'node:fs/promises'
import { homedir } from 'node:os'
import { basename, join, resolve, sep } from 'node:path'
import type { BrowserProfile } from '@shared/browserProfile'

export type LocalBrowserProfile = BrowserProfile & {
  database: string
  family: 'chromium' | 'firefox'
  keychainService?: string
}

export async function discoverBrowserProfiles(
  home = homedir(),
  platform = process.platform,
  appData = process.env.APPDATA,
): Promise<LocalBrowserProfile[]> {
  const profiles: LocalBrowserProfile[] = []
  if (platform === 'darwin') {
    for (const [browser, directory, keychainService] of [
      ['Chrome', 'Google/Chrome', 'Chrome Safe Storage'],
      ['Edge', 'Microsoft Edge', 'Microsoft Edge Safe Storage'],
    ]) {
      const root = join(home, 'Library/Application Support', directory)
      const state = await optionalText(join(root, 'Local State'))
      if (!state) {
        continue
      }
      let info: Record<string, { name?: unknown }>
      try {
        info = JSON.parse(state).profile?.info_cache ?? {}
      } catch {
        throw new Error(`${browser} profile details could not be read. Close the browser and try again.`)
      }
      for (const [folder, metadata] of Object.entries(info)) {
        if (basename(folder) !== folder || folder === '.' || folder === '..') {
          continue
        }
        for (const cookieFile of ['Network/Cookies', 'Cookies']) {
          const database = join(root, folder, cookieFile)
          if (await exists(database)) {
            profiles.push({
              id: profileID(database), browser,
              name: typeof metadata?.name === 'string' ? metadata.name : folder,
              database, family: 'chromium', keychainService,
            })
            break
          }
        }
      }
    }
  }
  const firefoxRoots = platform === 'darwin'
    ? [join(home, 'Library/Application Support/Firefox')]
    : platform === 'win32'
      ? [join(appData || join(home, 'AppData/Roaming'), 'Mozilla/Firefox')]
      : [join(home, '.mozilla/firefox'), join(home, 'snap/firefox/common/.mozilla/firefox'), join(home, '.var/app/org.mozilla.firefox/.mozilla/firefox')]
  for (const root of firefoxRoots) {
    const ini = await optionalText(join(root, 'profiles.ini'))
    if (!ini) {
      continue
    }
    for (const section of ini.split(/^\[/m).slice(1)) {
      if (!/^Profile\d+\]/.test(section)) {
        continue
      }
      const fields = new Map(section.split(/\r?\n/).slice(1).flatMap((line) => {
        const split = line.indexOf('=')
        return split < 0 ? [] : [[line.slice(0, split).trim(), line.slice(split + 1).trim()] as const]
      }))
      const folder = fields.get('Path')
      if (!folder) {
        continue
      }
      const directory = fields.get('IsRelative') === '0' ? resolve(folder) : resolve(root, folder.split('/').join(sep))
      const database = join(directory, 'cookies.sqlite')
      if (await exists(database)) {
        profiles.push({ id: profileID(database), browser: 'Firefox', name: fields.get('Name') || basename(directory), database, family: 'firefox' })
      }
    }
  }
  return [...new Map(profiles.map((profile) => [profile.id, profile])).values()]
}

function profileID(database: string): string {
  return createHash('sha256').update(database).digest('hex').slice(0, 24)
}

async function optionalText(file: string): Promise<string | undefined> {
  try {
    return await readFile(file, 'utf8')
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === 'ENOENT') {
      return undefined
    }
    throw new Error('Browser profiles could not be read. Check that Jaz has access to your browser data.', { cause: error })
  }
}

async function exists(file: string): Promise<boolean> {
  try {
    await access(file)
    return true
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === 'ENOENT') {
      return false
    }
    throw new Error('Browser profiles could not be read. Check that Jaz has access to your browser data.', { cause: error })
  }
}
