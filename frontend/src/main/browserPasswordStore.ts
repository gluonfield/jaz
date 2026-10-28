import { readFileSync, renameSync, writeFileSync } from 'node:fs'
import { app, safeStorage } from 'electron'
import { join } from 'node:path'

export type BrowserPassword = { origin: string; username: string; password: string }

export class BrowserPasswordStore {
  private records?: readonly BrowserPassword[]

  constructor(private readonly file = join(app.getPath('userData'), 'browser-passwords.enc')) {}

  get cached(): readonly BrowserPassword[] | undefined {
    return this.records
  }

  read(): readonly BrowserPassword[] {
    if (this.records) {
      return this.records
    }
    let encrypted: Buffer
    try {
      encrypted = readFileSync(this.file)
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code === 'ENOENT') {
        return this.records = []
      }
      throw new Error('Saved passwords could not be read.', { cause: error })
    }
    this.requireEncryption()
    try {
      const records = JSON.parse(safeStorage.decryptString(encrypted)) as BrowserPassword[]
      if (!Array.isArray(records) || records.some((entry) =>
        typeof entry?.origin !== 'string' || new URL(entry.origin).origin !== entry.origin || !entry.origin.startsWith('https://') ||
        typeof entry.username !== 'string' || typeof entry.password !== 'string')) {
        throw new Error('Invalid password file')
      }
      return this.records = records
    } catch {
      throw new Error('Saved passwords could not be unlocked. Check access to your system keychain.')
    }
  }

  save(password: BrowserPassword): void {
    const records = this.read().filter((entry) => entry.origin !== password.origin || entry.username !== password.username)
    this.write([...records, password])
  }

  import(passwords: BrowserPassword[]): number {
    const records = [...this.read()]
    const identities = new Set(records.map((entry) => JSON.stringify([entry.origin, entry.username])))
    const initial = records.length
    for (const password of passwords) {
      const identity = JSON.stringify([password.origin, password.username])
      if (!identities.has(identity)) {
        records.push(password)
        identities.add(identity)
      }
    }
    if (records.length > initial) {
      this.write(records)
    }
    return records.length - initial
  }

  remove(origin: string, username: string): void {
    this.write(this.read().filter((entry) => entry.origin !== origin || entry.username !== username))
  }

  private write(records: BrowserPassword[]): void {
    this.requireEncryption()
    try {
      const encrypted = safeStorage.encryptString(JSON.stringify(records))
      writeFileSync(this.file + '.tmp', encrypted, { mode: 0o600 })
      renameSync(this.file + '.tmp', this.file)
      this.records = records
    } catch {
      throw new Error('The password change could not be saved.')
    }
  }

  private requireEncryption(): void {
    if (!safeStorage.isEncryptionAvailable() || (process.platform === 'linux' && safeStorage.getSelectedStorageBackend() === 'basic_text')) {
      throw new Error('Saved passwords require an available system keychain.')
    }
  }
}

let sharedStore: BrowserPasswordStore | undefined

export function getBrowserPasswordStore(): BrowserPasswordStore {
  return sharedStore ??= new BrowserPasswordStore()
}
