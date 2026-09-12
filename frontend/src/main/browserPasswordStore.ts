import { readFileSync, renameSync, writeFileSync } from 'node:fs'
import { safeStorage } from 'electron'

export type BrowserPassword = { origin: string; username: string; password: string }

export class BrowserPasswordStore {
  constructor(private readonly file: string) {}

  read(): BrowserPassword[] {
    let encrypted: Buffer
    try {
      encrypted = readFileSync(this.file)
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code === 'ENOENT') {
        return []
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
      return records
    } catch {
      throw new Error('Saved passwords could not be unlocked. Check access to your system keychain.')
    }
  }

  save(password: BrowserPassword): void {
    const records = this.read().filter((entry) => entry.origin !== password.origin || entry.username !== password.username)
    this.write([...records, password])
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
    } catch {
      throw new Error('The password change could not be saved.')
    }
  }

  private requireEncryption(): void {
    if (!safeStorage.isEncryptionAvailable() || (process.platform === 'linux' && safeStorage.getSelectedStorageBackend() === 'basic_text')) {
      throw new Error('Password saving needs an available system keychain.')
    }
  }
}
