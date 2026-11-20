import { useLayoutEffect, useRef, useState, type ReactNode, type RefObject } from 'react'

const easing = 'cubic-bezier(.43, .15, .27, 1)'
const end = { '--tool-row-edge': 'calc(100% + 8px)' }

type Sweep = { frames: Keyframe[]; gap?: boolean; prefix?: string }

function rowSweep(content: Element, wide: boolean): Sweep {
  const begin = { '--tool-row-edge': '0%' }
  const command = wide ? content.querySelector('pre') : null
  if (!command) return { frames: [begin, end] }

  const bounds = content.getBoundingClientRect()
  if (!bounds.width) return { frames: [begin, end] }
  const range = document.createRange()
  range.selectNodeContents(command)
  const commandEnd = Math.min(range.getBoundingClientRect().right, command.getBoundingClientRect().right)
  const tail = content.lastElementChild
  const status = tail === command ? undefined : tail?.getBoundingClientRect()
  const edgeAt = (x: number) => `${100 * (x - bounds.left) / bounds.width}%`
  const finish = { '--tool-row-edge': `calc(${edgeAt(status?.right ?? commandEnd)} + 8px)` }
  if (!status || status.left - commandEnd <= 80) return { frames: [begin, finish] }

  return {
    frames: [
      { ...begin, easing },
      { '--tool-row-edge': `calc(${edgeAt(commandEnd)} + 8px)`, offset: .67, easing },
      { '--tool-row-edge': `calc(${edgeAt(status.left)} - 8px)`, offset: .86, easing },
      finish,
    ],
    gap: true,
  }
}

function labelSweep(label: HTMLSpanElement, from: number): Sweep | undefined {
  const text = label.firstElementChild?.firstChild
  if (!text) return
  const range = document.createRange()
  range.setStart(text, 0)
  range.setEnd(text, from)
  const bounds = label.getBoundingClientRect()
  const offset = range.getBoundingClientRect().right - bounds.left
  if (offset >= bounds.width - 8) return

  const prefix = `${offset}px`
  return { frames: [{ '--tool-row-edge': prefix }, end], prefix }
}

function playSweep(row: HTMLElement, measure: () => Sweep | undefined) {
  if (window.matchMedia('(prefers-reduced-motion: reduce)').matches ||
      document.documentElement.classList.contains('jaz-no-effects')) return
  const sweep = measure()
  if (!sweep) return

  const { frames, gap = false, prefix } = sweep
  const duration = gap ? 740 : 630
  if (prefix) row.style.setProperty('--tool-row-prefix', prefix)
  row.style.setProperty('--tool-row-duration', `${duration}ms`)
  row.classList.add('tool-row-revealing')
  row.classList.toggle('tool-row-crossing-gap', gap)
  const animation = row.animate(frames, { duration, easing: gap ? 'linear' : easing, fill: 'both' })
  const finish = () => {
    row.classList.remove('tool-row-revealing', 'tool-row-crossing-gap')
    row.style.removeProperty('--tool-row-duration')
    row.style.removeProperty('--tool-row-prefix')
    animation.onfinish = null
    animation.cancel()
  }
  animation.onfinish = finish
  return finish
}

export function ToolActivityReveal({
  children,
  reveal = false,
  wide = false,
  append,
  className,
}: {
  children: ReactNode
  reveal?: boolean
  wide?: boolean
  append?: { labelRef: RefObject<HTMLSpanElement | null>; from: number }
  className: string
}) {
  const ref = useRef<HTMLElement>(null)
  const stopSweep = useRef<(() => void) | undefined>(undefined)
  const [revealOnMount] = useState(reveal)
  const Tag = wide ? 'div' : 'span'

  useLayoutEffect(() => {
    const row = ref.current
    const content = row?.firstElementChild
    if (revealOnMount && row && content) stopSweep.current = playSweep(row, () => rowSweep(content, wide))
    return () => {
      stopSweep.current?.()
      stopSweep.current = undefined
    }
  }, [revealOnMount, wide])

  useLayoutEffect(() => {
    const label = append?.labelRef.current
    if (!append || !label) return

    stopSweep.current?.()
    stopSweep.current = playSweep(label, () => labelSweep(label, append.from))
  }, [append])

  if (!revealOnMount) return <Tag className={className}>{children}</Tag>

  return (
    <Tag ref={(element: HTMLElement | null) => { ref.current = element }} className="relative block min-w-0 max-w-full">
      <Tag className={className}>{children}</Tag>
    </Tag>
  )
}
