import { ipcMain, safeStorage } from 'electron'
import { isTrustedRendererURL } from '@main/permissions'

export function installCredentialEncryption(): void {
  ipcMain.on('jaz:credential-encryption', (event, operation: unknown, value: unknown) => {
    try {
      if (event.sender.getType() !== 'window' || event.senderFrame !== event.sender.mainFrame || !isTrustedRendererURL(event.senderFrame.url)) {
        throw new Error('Credentials require a trusted Jaz window')
      }
      if (typeof value !== 'string' || value.length > 16_384 || (operation !== 'encrypt' && operation !== 'decrypt')) {
        throw new Error('Invalid credential request')
      }
      if (!safeStorage.isEncryptionAvailable() || (process.platform === 'linux' && safeStorage.getSelectedStorageBackend() === 'basic_text')) {
        throw new Error('Saved connections require an available system keychain')
      }
      event.returnValue = { value: operation === 'encrypt'
        ? safeStorage.encryptString(value).toString('base64')
        : safeStorage.decryptString(Buffer.from(value, 'base64')) }
    } catch (error) {
      event.returnValue = { error: (error as Error).message }
    }
  })
}
