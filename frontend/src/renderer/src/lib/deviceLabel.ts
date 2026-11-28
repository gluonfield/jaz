export function localDeviceLabel(): string {
  if (/Mac/i.test(navigator.platform)) return 'this Mac'
  if (/Win/i.test(navigator.platform)) return 'this PC'
  return 'this computer'
}

export function revealInFolderLabel(): string {
  if (/Mac/i.test(navigator.platform)) return 'Show in Finder'
  if (/Win/i.test(navigator.platform)) return 'Show in File Explorer'
  return 'Show in Files'
}
