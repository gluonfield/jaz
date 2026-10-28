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
import { logMain } from '@main/diagnostics'

const PRIVACY = 'x-apple.systempreferences:com.apple.preference.security?'

// Electron exposes delivery events, not the current macOS notification grant.
let notifications: PermissionState = 'needed'

async function status(): Promise<SystemPermissionStatus> {
  const microphone = systemPreferences.getMediaAccessStatus('microphone')
  return {
    accessibility: systemPreferences.isTrustedAccessibilityClient(false) ? 'granted' : 'needed',
    screenRecording: systemPreferences.getMediaAccessStatus('screen') === 'granted' ? 'granted' : 'needed',
    microphone: microphone === 'granted' ? 'granted' : microphone === 'denied' || microphone === 'restricted' ? 'off' : 'needed',
    notifications: app.isPackaged ? notifications : 'unavailable',
  }
}

async function allow(permission: SystemPermission): Promise<void> {
  if (permission === 'accessibility' || permission === 'screenRecording') {
    const pane = permission === 'accessibility' ? 'Privacy_Accessibility' : 'Privacy_ScreenCapture'
    return guidePermission(permission, PRIVACY + pane, () => permission === 'accessibility'
      ? systemPreferences.isTrustedAccessibilityClient(false)
      : systemPreferences.getMediaAccessStatus('screen') === 'granted')
  }
  if (permission === 'microphone') {
    const microphone = systemPreferences.getMediaAccessStatus('microphone')
    if (microphone === 'denied' || microphone === 'restricted') {
      return shell.openExternal(PRIVACY + 'Privacy_Microphone')
    }
    await systemPreferences.askForMediaAccess('microphone')
    return
  }
  if (!app.isPackaged) {
    return
  }
  if (notifications === 'off') {
    await openNotificationSettings()
  }
  // An undecided permission is being asked for right now: give the user a
  // moment to answer the prompt.
  const shown = await sendTestNotification(notifications === 'needed' ? 8 : 1)
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
    return allow(permission).catch((error) => {
      logMain(`System permission action failed: ${permission}`, error)
      throw new Error('Couldn’t change this permission. Open System Settings → Privacy & Security, or Notifications, and select Jaz.')
    })
  })
}
