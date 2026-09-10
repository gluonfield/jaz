import assert from 'node:assert/strict'
import { createCipheriv, createHash, pbkdf2Sync } from 'node:crypto'
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { DatabaseSync } from 'node:sqlite'
import { BrowserWindow, session } from 'electron'
import { discoverBrowserProfiles } from '@main/browserProfiles'
import { importProfileCookies, listCookieSites } from '@main/browserCookies'
import { BrowserProfileImporter, installBrowserProfileImport } from '@main/browserProfileImport'
import { PREVIEW_PARTITION } from '@shared/preview'

export async function prepareProfileFixture(root: string): Promise<void> {
  const home = join(root, 'source-profiles')
  const chromeRoot = join(home, 'Library/Application Support/Google/Chrome')
  const profileRoot = join(chromeRoot, 'Default')
  await mkdir(profileRoot, { recursive: true })
  await writeFile(join(chromeRoot, 'Local State'), JSON.stringify({ profile: { info_cache: { Default: { name: 'Personal' }, '../../outside': { name: 'Invalid' } } } }))
  const database = join(profileRoot, 'Cookies')
  const db = new DatabaseSync(database)
  db.exec(`CREATE TABLE meta (key TEXT, value TEXT);
INSERT INTO meta VALUES ('version', '24');
CREATE TABLE cookies (
host_key TEXT, name TEXT, value TEXT, path TEXT, is_secure INTEGER, is_httponly INTEGER,
samesite INTEGER, expires_utc INTEGER, has_expires INTEGER, encrypted_value BLOB, top_frame_site_key TEXT
);`)
  const password = 'test-only-safe-storage'
  const key = pbkdf2Sync(password, 'saltysalt', 1003, 16, 'sha1')
  const encrypted = (host: string, value: string) => {
    const cipher = createCipheriv('aes-128-cbc', key, Buffer.alloc(16, 32))
    return Buffer.concat([Buffer.from('v10'), cipher.update(Buffer.concat([createHash('sha256').update(host).digest(), Buffer.from(value)])), cipher.final()])
  }
  const expires = Math.round((Date.now() / 1000 + 11644473600 + 86400) * 1_000_000)
  const insert = db.prepare('INSERT INTO cookies VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)')
  insert.run('127.0.0.1', 'jaz_import_fixture', '', '/', 0, 1, 1, expires, 1, encrypted('127.0.0.1', 'signed-in-fixture'), '')
  insert.run('not-selected.test', 'private_fixture', '', '/', 1, 1, 2, expires, 1, encrypted('not-selected.test', 'excluded'), '')
  insert.run('expired.test', 'expired', '', '/', 1, 0, 1, 1, 1, encrypted('expired.test', 'expired'), '')
  insert.run('partitioned.test', 'partitioned', '', '/', 1, 0, 1, expires, 1, encrypted('partitioned.test', 'partitioned'), 'https://partition.test')
  insert.run('bad-hash.test', 'bad_hash', '', '/', 1, 0, 1, expires, 1, encrypted('different.test', 'wrong-host'), '')
  insert.run('.host-only.test', 'domain', 'domain-value', '/limited', 1, 1, 2, 0, 0, Buffer.alloc(0), '')
  insert.run('host-only.test', '__Host-exact', 'host-value', '/', 1, 1, 1, expires, 1, Buffer.alloc(0), '')
  db.close()
  const original = await readFile(database)
  const profiles = await discoverBrowserProfiles(home, 'darwin')
  assert.equal(profiles.length, 1)
  assert.equal(profiles[0].name, 'Personal')
  assert.equal(profiles[0].browser, 'Chrome')
  const chrome = profiles[0]
  const sites = listCookieSites(chrome)
  assert.equal(sites.find((site) => site.domain === 'host-only.test')?.cookies, 2)
  assert(!sites.some((site) => site.domain === 'expired.test' || site.domain === 'partitioned.test'))
  assert(!JSON.stringify(sites).includes('signed-in-fixture'))
  const jar = session.fromPartition('profile-contract-' + process.pid).cookies
  await assert.rejects(importProfileCookies(chrome, [], (cookie) => jar.set(cookie)), /Choose the sites/)
  await assert.rejects(importProfileCookies(chrome, ['127.0.0.1'], (cookie) => jar.set(cookie), async () => {
    throw new Error('Keychain denied')
  }), /Keychain denied/)
  assert.equal((await jar.get({})).length, 0)
  const result = await importProfileCookies(chrome, ['127.0.0.1', 'bad-hash.test', 'host-only.test'], (cookie) => jar.set(cookie), async () => password)
  assert.deepEqual(result, { imported: 3, failed: 1 })
  const cookies = await jar.get({})
  assert.equal(cookies.find((cookie) => cookie.name === 'jaz_import_fixture')?.value, 'signed-in-fixture')
  assert(cookies.find((cookie) => cookie.name === 'jaz_import_fixture')?.httpOnly)
  assert(cookies.find((cookie) => cookie.name === '__Host-exact')?.hostOnly)
  assert.equal(cookies.find((cookie) => cookie.name === 'domain')?.hostOnly, false)
  assert.equal(cookies.find((cookie) => cookie.name === 'domain')?.path, '/limited')
  assert.equal(cookies.find((cookie) => cookie.name === 'domain')?.sameSite, 'strict')
  assert(!cookies.some((cookie) => cookie.name === 'private_fixture' || cookie.name === 'bad_hash'))
  assert.deepEqual(await readFile(database), original)

  const firefoxRoot = join(home, 'Library/Application Support/Firefox')
  await mkdir(join(firefoxRoot, 'Profiles/fixture'), { recursive: true })
  await writeFile(join(firefoxRoot, 'profiles.ini'), '[Profile0]\nName=Work\nIsRelative=1\nPath=Profiles/fixture\n')
  const firefoxDatabase = join(firefoxRoot, 'Profiles/fixture/cookies.sqlite')
  const firefox = new DatabaseSync(firefoxDatabase)
  firefox.exec('CREATE TABLE moz_cookies (host TEXT, name TEXT, value TEXT, path TEXT, isSecure INTEGER, isHttpOnly INTEGER, sameSite INTEGER, expiry INTEGER, originAttributes TEXT)')
  const firefoxInsert = firefox.prepare('INSERT INTO moz_cookies VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)')
  firefoxInsert.run('firefox.test', 'firefox', 'firefox-value', '/', 1, 1, 2, Math.round(Date.now() / 1000 + 86400), '')
  firefoxInsert.run('container.test', 'container', 'excluded', '/', 1, 1, 2, Math.round(Date.now() / 1000 + 86400), '^userContextId=2')
  firefox.close()
  const found = await discoverBrowserProfiles(home, 'darwin')
  assert.equal(found.length, 2)
  const firefoxProfile = found.find((profile) => profile.family === 'firefox')!
  assert.deepEqual(listCookieSites(firefoxProfile), [{ domain: 'firefox.test', cookies: 1 }])
  const firefoxResult = await importProfileCookies(firefoxProfile, ['firefox.test'], (cookie) => jar.set(cookie), async () => {
    throw new Error('Firefox must not access Keychain')
  })
  assert.equal(firefoxResult.imported, 1)
  const importer = new BrowserProfileImporter(async () => found, async () => password)
  assert.equal(await importer.dismissed(), false)
  await assert.rejects(importer.import('../../arbitrary-path', ['127.0.0.1']), /no longer available/)
  assert.equal((await session.fromPartition(PREVIEW_PARTITION).cookies.get({})).length, 0)
  installBrowserProfileImport(importer)
}

export async function assertUntrustedProfileCaller(root: string): Promise<void> {
  const untrusted = new BrowserWindow({ show: false, webPreferences: { preload: join(root, 'preload.js'), sandbox: true, contextIsolation: true } })
  try {
    await untrusted.loadURL('data:text/html,<title>Untrusted import caller</title>')
    const rejected = await untrusted.webContents.executeJavaScript("window.jaz.browserProfiles.list().then(() => false, error => error.message.includes('Browser imports must be started from Jaz.'))")
    assert.equal(rejected, true)
  } finally {
    untrusted.destroy()
  }
}
