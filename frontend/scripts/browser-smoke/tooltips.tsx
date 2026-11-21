import { createRoot } from 'react-dom/client'
import { TooltipLayer } from '@/components/ui/Tooltip'

// A titled control shows the app tooltip with its shortcut in place of the
// browser's: the title is parked while the pointer is on it and returns after,
// the next control opens without waiting, and a title that only repeats the
// visible text shows nothing.
export async function exerciseTooltips(): Promise<void> {
  const element = document.createElement('div')
  document.body.append(element)
  const root = createRoot(element)
  root.render(
    <>
      <button type="button" title="Hide sidebar" aria-keyshortcuts="Meta+S">A</button>
      <button type="button" disabled title="Go back">B</button>
      <button type="button" title="Dismiss">Dismiss</button>
      <TooltipLayer />
    </>,
  )
  const until = async (check: () => boolean) => {
    const deadline = Date.now() + 3000
    while (!check()) {
      if (Date.now() > deadline) {
        throw new Error('Tooltips: ' + check.toString())
      }
      await new Promise((resolve) => setTimeout(resolve, 10))
    }
  }
  const tooltip = () => document.querySelector('[role="tooltip"]')?.textContent
  const over = (target: Element) => target.dispatchEvent(new PointerEvent('pointerover', { bubbles: true, pointerType: 'mouse' }))
  try {
    await until(() => element.querySelectorAll('button').length === 3)
    const [sidebar, back, dismiss] = element.querySelectorAll('button')
    over(sidebar)
    if (sidebar.hasAttribute('title')) {
      throw new Error('A hovered control kept its native title')
    }
    await until(() => tooltip() === 'Hide sidebar⌘S')
    over(back)
    await new Promise((resolve) => setTimeout(resolve, 50))
    if (tooltip() !== 'Go back' || sidebar.title !== 'Hide sidebar') {
      throw new Error(`Moving to a disabled control showed ${tooltip()} and left the first title ${sidebar.title}`)
    }
    over(dismiss)
    await new Promise((resolve) => setTimeout(resolve, 50))
    if (tooltip()) {
      throw new Error('A title repeating its visible label opened a tooltip')
    }
    over(back)
    await until(() => tooltip() === 'Go back')
    back.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, pointerType: 'mouse' }))
    await until(() => !tooltip())
    over(document.body)
    if (back.title !== 'Go back') {
      throw new Error('Leaving a control did not return its title')
    }
  } finally {
    root.unmount()
    element.remove()
  }
}
