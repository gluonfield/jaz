import { flushSync } from 'react-dom'
import { createRoot } from 'react-dom/client'
import { Button } from '@/components/ui/Button'
import { applyPreset, getColorTheme, setColorTheme, THEME_PRESETS } from '@/lib/appearanceScheme'

// Resolve any CSS colour, oklch included, to sRGB by painting it.
function luminance(color: string): number {
  const context = document.createElement('canvas').getContext('2d')!
  context.fillStyle = color
  context.fillRect(0, 0, 1, 1)
  const [r, g, b] = [...context.getImageData(0, 0, 1, 1).data.slice(0, 3)].map((channel) => {
    const c = channel / 255
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
  })
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

export async function exerciseAccentText(): Promise<void> {
  const dark = document.documentElement.classList.contains('dark')
  const theme = getColorTheme()
  const element = document.createElement('div')
  document.body.append(element)
  const root = createRoot(element)
  flushSync(() => root.render(<Button variant="primary" style={{ transition: 'none' }}>Send</Button>))
  const button = element.querySelector('button')!
  try {
    for (const preset of THEME_PRESETS) {
      for (const mode of ['light', 'dark'] as const) {
        applyPreset(preset)
        document.documentElement.classList.toggle('dark', mode === 'dark')
        await new Promise(requestAnimationFrame)
        const style = getComputedStyle(button)
        const [text, fill] = [luminance(style.color), luminance(style.backgroundColor)]
        const contrast = (Math.max(text, fill) + 0.05) / (Math.min(text, fill) + 0.05)
        if (contrast < 4.5) {
          throw new Error(`${preset.label} ${mode} buttons read at ${contrast.toFixed(2)}:1, under 4.5:1`)
        }
      }
    }
  } finally {
    setColorTheme(theme)
    document.documentElement.classList.toggle('dark', dark)
    root.unmount()
    element.remove()
  }
}
