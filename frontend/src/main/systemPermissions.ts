import { app, ipcMain, shell, systemPreferences } from 'electron'
import {
  SYSTEM_PERMISSIONS,
  SYSTEM_PERMISSIONS_CHANNEL,
  type PermissionState,
  type SystemPermission,
  type SystemPermissionStatus,
} from '@shared/systemPermissions'
import { openNotificationSettings, sendTestNotification } from '@main/notifications'
import { guidePermission } from '@main/permissionGuide'
import { trustedWindow } from '@main/permissions'

const PRIVACY = 'x-apple.systempreferences:com.apple.preference.security?'
const loadDriver = () => import('@trycua/cua-driver')

// macOS reports notification permission only by showing or refusing a
// notification, so this is what Jaz last observed in this process.
let notifications: PermissionState = 'needed'

async function status(): Promise<SystemPermissionStatus> {
  const computer = (await loadDriver()).currentMacOsPermissionStatus()
  const microphone = systemPreferences.getMediaAccessStatus('microphone')
  return {
    accessibility: computer.accessibility ? 'granted' : 'needed',
    screenRecording: computer.screenRecording ? 'granted' : 'needed',
    microphone: microphone === 'granted' ? 'granted' : microphone === 'denied' || microphone === 'restricted' ? 'off' : 'needed',
    notifications: app.isPackaged ? notifications : 'unavailable',
  }
}

async function allow(permission: SystemPermission): Promise<void> {
  if (permission === 'accessibility' || permission === 'screenRecording') {
    const sdk = await loadDriver()
    const pane = permission === 'accessibility' ? 'Privacy_Accessibility' : 'Privacy_ScreenCapture'
    return guidePermission(permission, PRIVACY + pane, () => sdk.currentMacOsPermissionStatus()[permission])
  }
  if (permission === 'microphone') {
    if ((await status()).microphone === 'off') {
      return shell.openExternal(PRIVACY + 'Privacy_Microphone')
    }
    await systemPreferences.askForMediaAccess('microphone')
    return
  }
  if (!app.isPackaged) {
    return
  }
  // An undecided permission is being asked for right now: give the user a
  // moment to answer the prompt.
  const shown = await sendTestNotification(notifications === 'needed' ? 8 : 1)
  if (!shown && notifications === 'off') {
    await openNotificationSettings()
  }
  notifications = shown ? 'granted' : 'off'
}

export function installSystemPermissions(): void {
  ipcMain.handle(SYSTEM_PERMISSIONS_CHANNEL + 'status', (event) => {
    trustedWindow(event)
    return status()
  })
  ipcMain.handle(SYSTEM_PERMISSIONS_CHANNEL + 'allow', (event, permission: SystemPermission) => {
    trustedWindow(event)
    if (!SYSTEM_PERMISSIONS.includes(permission)) {
      throw new Error('Invalid system permission')
    }
    return allow(permission)
  })
}
