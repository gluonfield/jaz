import { readFileSync, renameSync, writeFileSync } from 'node:fs'
import { app } from 'electron'
import { join } from 'node:path'
import { passwordOrigin } from '@shared/browserPasswords'
import { getDesktopSecretStore, SecretStoreError, type DesktopSecretStore } from '@main/secretStore'

export type BrowserPassword = { origin: string; username: string; password: string }

export class BrowserPasswordStore {
  private records?: readonly BrowserPassword[]

  constructor(
    private readonly file = join(app.getPath('userData'), 'browser-passwords.enc'),
    private readonly secrets: DesktopSecretStore = getDesktopSecretStore(),
  ) {}

  get cached(): readonly BrowserPassword[] | undefined {
    return this.records
  }

  tryAutoUnlock(): boolean {
    if (this.records) {
      return true
    }
    let encrypted: Buffer | null
    try {
      encrypted = this.readEncryptedFile()
    } catch {
      return false
    }
    if (encrypted === null) {
      this.records = []
      return true
    }
    if (!this.secrets.canAutoUnlock(encrypted)) {
      return false
    }
    try {
      this.unlockBuffer(encrypted, { allowLegacy: false })
      return true
    } catch {
      return false
    }
  }

  read(options?: { allowLegacy?: boolean }): readonly BrowserPassword[] {
    if (this.records) {
      return this.records
    }
    const encrypted = this.readEncryptedFile()
    if (encrypted === null) {
      return this.records = []
    }
    return this.unlockBuffer(encrypted, options)
  }

  save(password: BrowserPassword): void {
    const records = this.read({ allowLegacy: true }).filter((entry) => entry.origin !== password.origin || entry.username !== password.username)
    this.write([...records, password])
  }

  import(passwords: BrowserPassword[]): number {
    const records = [...this.read({ allowLegacy: true })]
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
    this.write(this.read({ allowLegacy: true }).filter((entry) => entry.origin !== origin || entry.username !== username))
  }

  private readEncryptedFile(): Buffer | null {
    try {
      return readFileSync(this.file)
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code === 'ENOENT') {
        return null
      }
      throw new Error('Saved passwords could not be read.', { cause: error })
    }
  }

  private unlockBuffer(encrypted: Buffer, options?: { allowLegacy?: boolean }): readonly BrowserPassword[] {
    let plaintext: string
    let migratedFromLegacy = false
    try {
      const opened = this.secrets.decryptBuffer(encrypted, options)
      plaintext = opened.plaintext
      migratedFromLegacy = opened.migratedFromLegacy
    } catch (error) {
      if (error instanceof SecretStoreError) {
        throw error
      }
      throw new Error('Saved passwords could not be unlocked. Check access to your system keychain.')
    }
    let records: BrowserPassword[]
    try {
      records = JSON.parse(plaintext) as BrowserPassword[]
      if (!Array.isArray(records) || records.some((entry) =>
        typeof entry?.origin !== 'string' || passwordOrigin(entry.origin) !== entry.origin ||
        typeof entry.username !== 'string' || typeof entry.password !== 'string')) {
        throw new Error('Invalid password file')
      }
    } catch {
      throw new Error('Saved passwords could not be unlocked. Check access to your system keychain.')
    }
    this.records = records
    if (migratedFromLegacy) {
      try {
        this.write(records)
      } catch {
        // Keep unlocked records usable even if opportunistic on-disk migration fails.
      }
    }
    return this.records
  }

  private write(records: BrowserPassword[]): void {
    let encrypted: Buffer
    try {
      encrypted = this.secrets.encryptBuffer(JSON.stringify(records))
    } catch (error) {
      if (error instanceof SecretStoreError) {
        throw error
      }
      throw new Error('The password change could not be saved.')
    }
    try {
      writeFileSync(this.file + '.tmp', encrypted, { mode: 0o600 })
      renameSync(this.file + '.tmp', this.file)
      this.records = records
    } catch {
      throw new Error('The password change could not be saved.')
    }
  }
}

let sharedStore: BrowserPasswordStore | undefined

export function getBrowserPasswordStore(): BrowserPasswordStore {
  return sharedStore ??= new BrowserPasswordStore()
}
