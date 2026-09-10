import { execFile } from 'node:child_process'
import { createDecipheriv, createHash, pbkdf2Sync, timingSafeEqual } from 'node:crypto'
import { DatabaseSync } from 'node:sqlite'
import { promisify } from 'node:util'
import type { CookiesSetDetails } from 'electron'
import type { BrowserCookieSite, BrowserImportResult } from '@shared/browserProfile'
import type { LocalBrowserProfile } from '@main/browserProfiles'

const execute = promisify(execFile)
const CHROME_EPOCH_SECONDS = 11644473600

type CookieRow = {
  domain: string
  name: string
  value: string
  path: string
  secure: number
  httpOnly: number
  sameSite: number
  expires: number
  persistent: number
  encrypted?: Uint8Array
}

function readCookies(profile: LocalBrowserProfile, domains: string[]): { rows: CookieRow[]; version: number } {
  const db = new DatabaseSync(profile.database, { readOnly: true, timeout: 1000 })
  try {
    const chromium = profile.family === 'chromium'
    const column = chromium ? 'host_key' : 'host'
    const filter = ` AND ${column} IN (${domains.map(() => '?').join(',')})`
    const statement = db.prepare(chromium
      ? `SELECT host_key AS domain, name, value, path, is_secure AS secure, is_httponly AS httpOnly,
          samesite AS sameSite, expires_utc AS expires, has_expires AS persistent, encrypted_value AS encrypted
          FROM cookies WHERE top_frame_site_key = ''${filter}`
      : `SELECT host AS domain, name, value, path, isSecure AS secure, isHttpOnly AS httpOnly,
          sameSite, expiry AS expires, 1 AS persistent FROM moz_cookies WHERE originAttributes = ''${filter}`)
    statement.setReadBigInts(true)
    const rows = statement.all(...domains).map((row) => ({
      ...row,
      expires: chromium ? Number(row.expires) / 1_000_000 - CHROME_EPOCH_SECONDS : Number(row.expires),
      secure: Number(row.secure), httpOnly: Number(row.httpOnly), sameSite: Number(row.sameSite), persistent: Number(row.persistent),
    })) as CookieRow[]
    const version = chromium ? Number(db.prepare("SELECT value FROM meta WHERE key = 'version'").get()?.value) : 0
    if (chromium && !Number.isFinite(version)) {
      throw new Error('Missing cookie database version')
    }
    return { rows: rows.filter((row) => !row.persistent || row.expires > Date.now() / 1000), version }
  } finally {
    db.close()
  }
}

export function listCookieSites(profile: LocalBrowserProfile): BrowserCookieSite[] {
  const db = new DatabaseSync(profile.database, { readOnly: true, timeout: 1000 })
  try {
    const statement = db.prepare(profile.family === 'chromium'
      ? "SELECT ltrim(host_key, '.') AS domain, count(*) AS cookies FROM cookies WHERE top_frame_site_key = '' AND (has_expires = 0 OR expires_utc > ?) GROUP BY domain ORDER BY domain"
      : "SELECT ltrim(host, '.') AS domain, count(*) AS cookies FROM moz_cookies WHERE originAttributes = '' AND expiry > ? GROUP BY domain ORDER BY domain")
    const now = Date.now() / 1000
    return statement.all(profile.family === 'chromium' ? (now + CHROME_EPOCH_SECONDS) * 1_000_000 : now).map((row) => ({ domain: String(row.domain), cookies: Number(row.cookies) }))
  } finally {
    db.close()
  }
}

export async function importProfileCookies(
  profile: LocalBrowserProfile,
  domains: string[],
  setCookie: (cookie: CookiesSetDetails) => Promise<void>,
  password: (service: string) => Promise<string> = readKeychainPassword,
): Promise<BrowserImportResult> {
  if (!Array.isArray(domains) || !domains.length || domains.length > 2000 || domains.some((domain) => typeof domain !== 'string' || !domain || domain.length > 253)) {
    throw new Error('Choose the sites to import.')
  }
  const selected = [...new Set(domains)]
  const { rows, version } = readCookies(profile, selected.flatMap((domain) => [domain, '.' + domain]))
  if (!rows.length) {
    throw new Error('No current cookies found for the selected sites. Choose the profile again to refresh the list.')
  }
  let key: Buffer | undefined
  if (rows.some((row) => row.encrypted?.length)) {
    if (!profile.keychainService) {
      throw new Error('Encrypted cookies are unsupported for this browser.')
    }
    key = pbkdf2Sync(await password(profile.keychainService), 'saltysalt', 1003, 16, 'sha1')
  }
  const result = { imported: 0, failed: 0 }
  try {
    for (const row of rows) {
      try {
        const value = row.encrypted?.length ? decryptCookie(row, version, key!) : row.value
        const host = row.domain.replace(/^\./, '')
        const url = new URL(`${row.secure ? 'https' : 'http'}://${host}`)
        if (url.hostname !== host || url.username || url.password || url.port) {
          throw new Error('Invalid cookie domain')
        }
        await setCookie({
          url: url.origin + row.path, name: row.name, value, path: row.path,
          ...(row.domain.startsWith('.') ? { domain: row.domain } : {}),
          secure: Boolean(row.secure), httpOnly: Boolean(row.httpOnly),
          sameSite: row.sameSite === 0 ? 'no_restriction' : row.sameSite === 1 ? 'lax' : row.sameSite === 2 ? 'strict' : 'unspecified',
          ...(row.persistent ? { expirationDate: row.expires } : {}),
        })
        result.imported += 1
      } catch {
        result.failed += 1
      }
    }
    return result
  } finally {
    key?.fill(0)
  }
}

function decryptCookie(row: CookieRow, version: number, key: Buffer): string {
  const encrypted = Buffer.from(row.encrypted!)
  if (encrypted.subarray(0, 3).toString() !== 'v10') {
    throw new Error('Unsupported cookie encryption')
  }
  const decipher = createDecipheriv('aes-128-cbc', key, Buffer.alloc(16, 32))
  let value = Buffer.concat([decipher.update(encrypted.subarray(3)), decipher.final()])
  // Chromium schema 24 binds each encrypted value to the SHA-256 of its exact host.
  if (version >= 24) {
    const hash = createHash('sha256').update(row.domain).digest()
    if (value.length < hash.length || !timingSafeEqual(value.subarray(0, hash.length), hash)) {
      throw new Error('Cookie host verification failed')
    }
    value = value.subarray(hash.length)
  }
  return value.toString('utf8')
}

async function readKeychainPassword(service: string): Promise<string> {
  if (process.platform !== 'darwin') {
    throw new Error('This browser supports cookie import on macOS only.')
  }
  try {
    const { stdout } = await execute('/usr/bin/security', ['find-generic-password', '-w', '-a', service.replace(/ Safe Storage$/, ''), '-s', service], { timeout: 30000, maxBuffer: 8192 })
    return stdout.replace(/\r?\n$/, '')
  } catch {
    throw new Error('Keychain access was not granted. Allow access to the selected browser’s Safe Storage key, then try again.')
  }
}
