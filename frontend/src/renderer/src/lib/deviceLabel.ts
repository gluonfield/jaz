export function localDeviceLabel(): string {
  if (/Mac/i.test(navigator.platform)) return 'this Mac'
  if (/Win/i.test(navigator.platform)) return 'this PC'
  return 'this computer'
}
