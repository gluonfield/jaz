export const SYSTEM_PERMISSIONS = ['accessibility', 'screenRecording', 'microphone', 'notifications'] as const

export type SystemPermission = (typeof SYSTEM_PERMISSIONS)[number]

// granted: allowed. needed: undecided, or unknown until Jaz tries; Allow asks.
// off: refused; Allow opens System Settings. unavailable: this build cannot
// hold it (macOS refuses notifications from ad-hoc signed development builds).
export type PermissionState = 'granted' | 'needed' | 'off' | 'unavailable'

export type SystemPermissionStatus = Record<SystemPermission, PermissionState>

export interface SystemPermissionsAPI {
  status(): Promise<SystemPermissionStatus>
  allow(permission: SystemPermission): Promise<void>
}

// The panel docked to System Settings for the permissions granted by dragging
// Jaz into a list.
export type GuidedPermission = Extract<SystemPermission, 'accessibility' | 'screenRecording'>

export interface PermissionGuideAPI {
  drag(): void
  close(): void
}

export const SYSTEM_PERMISSIONS_CHANNEL = 'jaz:system-permissions:'
export const PERMISSION_GUIDE_CHANNEL = 'jaz:permission-guide:'

export const SYSTEM_PERMISSION_TITLES: Record<SystemPermission, string> = {
  accessibility: 'Accessibility',
  screenRecording: 'Screen Recording',
  microphone: 'Microphone',
  notifications: 'Notifications',
}
