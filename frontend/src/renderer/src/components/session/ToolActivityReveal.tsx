import { useLayoutEffect, useRef, useState, type ReactNode } from 'react'

const easing = 'cubic-bezier(.43, .15, .27, 1)'

function sweep(content: Element, wide: boolean) {
  const begin = { '--tool-row-edge': '0%' }
  const end = { '--tool-row-edge': 'calc(100% + 8px)' }
  const command = wide ? content.querySelector('pre') : null
  if (!command) return { frames: [begin, end], duration: 630, gap: false }

  const bounds = content.getBoundingClientRect()
  if (!bounds.width) return { frames: [begin, end], duration: 630, gap: false }
  const range = document.createRange()
  range.selectNodeContents(command)
  const commandEnd = Math.min(range.getBoundingClientRect().right, command.getBoundingClientRect().right)
  const tail = content.lastElementChild
  const status = tail === command ? undefined : tail?.getBoundingClientRect()
  const edgeAt = (x: number) => `${100 * (x - bounds.left) / bounds.width}%`
  const finish = { '--tool-row-edge': `calc(${edgeAt(status?.right ?? commandEnd)} + 8px)` }
  if (!status || status.left - commandEnd <= 80) return { frames: [begin, finish], duration: 630, gap: false }

  return {
    frames: [
      { ...begin, easing },
      { '--tool-row-edge': `calc(${edgeAt(commandEnd)} + 8px)`, offset: .67, easing },
      { '--tool-row-edge': `calc(${edgeAt(status.left)} - 8px)`, offset: .86, easing },
      finish,
    ],
    duration: 740,
    gap: true,
  }
}

export function ToolActivityReveal({
  children,
  reveal = false,
  wide = false,
  className,
}: {
  children: ReactNode
  reveal?: boolean
  wide?: boolean
  className: string
}) {
  const ref = useRef<HTMLElement>(null)
  const [revealOnMount] = useState(reveal)
  const Tag = wide ? 'div' : 'span'

  useLayoutEffect(() => {
    const row = ref.current
    const content = row?.firstElementChild
    if (!revealOnMount || !row || !content || window.matchMedia('(prefers-reduced-motion: reduce)').matches ||
        document.documentElement.classList.contains('jaz-no-effects')) return

    const { frames, duration, gap } = sweep(content, wide)
    row.style.setProperty('--tool-row-duration', `${duration}ms`)
    row.classList.add('tool-row-revealing')
    row.classList.toggle('tool-row-crossing-gap', gap)
    const animation = row.animate(frames, { duration, easing: gap ? 'linear' : easing, fill: 'both' })
    const finish = () => {
      row.classList.remove('tool-row-revealing', 'tool-row-crossing-gap')
      row.style.removeProperty('--tool-row-duration')
      animation.cancel()
    }
    animation.onfinish = finish
    return finish
  }, [revealOnMount, wide])

  if (!revealOnMount) return <Tag className={className}>{children}</Tag>

  return (
    <Tag ref={(element: HTMLElement | null) => { ref.current = element }} className="relative block min-w-0 max-w-full">
      <Tag className={className}>{children}</Tag>
    </Tag>
  )
}
