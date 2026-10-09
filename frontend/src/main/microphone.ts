import { ipcMain, shell, systemPreferences } from 'electron'

export function registerMicrophoneAccess(): void {
  ipcMain.handle('jaz:microphone:status', () => systemPreferences.getMediaAccessStatus('microphone'))
  // macOS prompts only once; after a refusal Allow opens the privacy list.
  ipcMain.handle('jaz:microphone:allow', async () => {
    if (systemPreferences.getMediaAccessStatus('microphone') === 'denied') {
      await shell.openExternal('x-apple.systempreferences:com.apple.preference.security?Privacy_Microphone')
      return false
    }
    return systemPreferences.askForMediaAccess('microphone')
  })
}
