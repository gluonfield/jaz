import { createRoot } from 'react-dom/client'
import { Button } from '@/components/ui/Button'
import { applyPreset, resetScheme, THEME_PRESETS } from '@/lib/appearanceScheme'

export async function exerciseAccentText(): Promise<void> {
  const dark = document.documentElement.classList.contains('dark')
  const element = document.createElement('div')
  document.body.append(element)
  const root = createRoot(element)
  root.render(<Button variant="primary" style={{ transition: 'none' }}>Send</Button>)
  const expectText = async (id: string, mode: 'light' | 'dark', color: string) => {
    applyPreset(mode, THEME_PRESETS.find((preset) => preset.id === id)!)
    document.documentElement.classList.toggle('dark', mode === 'dark')
    await new Promise(requestAnimationFrame)
    const actual = getComputedStyle(element.querySelector('button')!).color
    if (actual !== color) {
      throw new Error(`${id} ${mode} accent buttons use ${actual}, not ${color}`)
    }
  }
  try {
    await expectText('codex', 'light', 'rgb(255, 255, 255)')
    await expectText('codex', 'dark', 'rgb(255, 255, 255)')
    await expectText('ayu', 'light', 'rgb(16, 19, 26)')
  } finally {
    resetScheme()
    document.documentElement.classList.toggle('dark', dark)
    root.unmount()
    element.remove()
  }
}
