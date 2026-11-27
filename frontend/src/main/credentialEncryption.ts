import { ipcMain } from 'electron'
import { trustedWindow } from '@main/permissions'
import { getDesktopSecretStore, type DesktopSecretStore } from '@main/secretStore'

export function installCredentialEncryption(store: DesktopSecretStore = getDesktopSecretStore()): void {
  ipcMain.on('jaz:credential-encryption', (event, operation: unknown, value: unknown, allowLegacy?: unknown) => {
    try {
      trustedWindow(event)
      if (typeof value !== 'string' || value.length > 16_384 || (operation !== 'encrypt' && operation !== 'decrypt')) {
        throw new Error('Invalid credential request')
      }
      event.returnValue = {
        value: operation === 'encrypt'
          ? store.encryptText(value)
          : store.decryptText(value, { allowLegacy: allowLegacy === true }).plaintext,
      }
    } catch (error) {
      event.returnValue = { error: (error as Error).message }
    }
  })
}
