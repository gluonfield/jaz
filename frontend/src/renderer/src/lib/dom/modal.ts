export function modalDialogOpen(): boolean {
  return Boolean(document.querySelector('[role="dialog"][aria-modal="true"]'))
}
