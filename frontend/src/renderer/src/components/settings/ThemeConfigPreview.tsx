import type { ReactNode } from 'react'
import { type ColorScheme, useScheme } from '@/lib/appearanceScheme'

// A `ThemeConfig` snippet shown as a git-style diff between the live light and
// dark schemes: the left pane is the "removed" side (red), the right is "added"
// (green), and value lines that differ get the changed-line treatment. Syntax is
// hand-coloured (small fixed snippet).
const KW = 'text-[#8b5cf6]'
const TY = 'text-[#0d9aa6]'
const ST = 'text-[#3f9142]'
const NU = 'text-[#c2740a]'

function DiffPane({
  scheme,
  side,
  changed,
}: {
  scheme: ColorScheme
  side: 'del' | 'add'
  changed: Record<'accent' | 'background' | 'foreground' | 'contrast', boolean>
}) {
  const del = side === 'del'
  const sign = del ? '-' : '+'
  const lineBg = del ? 'bg-rose-500/10' : 'bg-emerald-500/10'
  const signColor = del ? 'text-rose-500' : 'text-emerald-500'
  const rows: [string, boolean, ReactNode][] = [
    ['head', false, <><span className={KW}>const</span> themePreview: <span className={TY}>ThemeConfig</span> = {'{'}</>],
    ['accent', changed.accent, <>{'  '}accent: <span className={ST}>&quot;{scheme.accent}&quot;</span>,</>],
    ['background', changed.background, <>{'  '}background: <span className={ST}>&quot;{scheme.background}&quot;</span>,</>],
    ['foreground', changed.foreground, <>{'  '}foreground: <span className={ST}>&quot;{scheme.foreground}&quot;</span>,</>],
    ['contrast', changed.contrast, <>{'  '}contrast: <span className={NU}>{scheme.contrast}</span>,</>],
    ['close', false, <>{'}'};</>],
  ]
  return (
    <div className={`border-l-2 py-2 ${del ? 'border-rose-400/50' : 'border-emerald-400/50'}`}>
      {rows.map(([key, isChanged, node], i) => (
        <div key={key} className={`flex ${isChanged ? lineBg : ''}`}>
          <span className={`w-4 shrink-0 select-none text-center ${signColor}`}>{isChanged ? sign : ''}</span>
          <span className="w-7 shrink-0 select-none pr-2 text-right text-ink-3/50">{i + 1}</span>
          <span className="whitespace-pre pr-3 text-ink-2">{node}</span>
        </div>
      ))}
    </div>
  )
}

export function ThemeConfigPreview() {
  const { light, dark } = useScheme()
  const changed = {
    accent: light.accent !== dark.accent,
    background: light.background !== dark.background,
    foreground: light.foreground !== dark.foreground,
    contrast: light.contrast !== dark.contrast,
  }
  return (
    <div className="grid grid-cols-2 overflow-hidden rounded-control bg-surface font-mono text-[11px] leading-[1.7] ring-1 ring-border/60">
      <DiffPane scheme={light} side="del" changed={changed} />
      <DiffPane scheme={dark} side="add" changed={changed} />
    </div>
  )
}
