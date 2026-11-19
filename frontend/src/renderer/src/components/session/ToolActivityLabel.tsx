import { useLayoutEffect, useRef, useState } from 'react'

export function ToolActivityLabel({
  text,
  reveal = false,
  className = 'block truncate',
}: {
  text: string
  reveal?: boolean
  className?: string
}) {
  const ref = useRef<HTMLSpanElement>(null)
  const [revealOnMount] = useState(reveal)

  useLayoutEffect(() => {
    const label = ref.current
    if (!revealOnMount || !label || window.matchMedia('(prefers-reduced-motion: reduce)').matches ||
        document.documentElement.classList.contains('jaz-no-effects')) return

    label.classList.add('tool-label-revealing')
    const animation = label.animate(
      [{ '--tool-label-edge': '0%' }, { '--tool-label-edge': 'calc(100% + 6px)' }],
      { duration: 500, easing: 'cubic-bezier(.24, .62, .25, 1)', fill: 'both' },
    )
    const finish = () => {
      label.classList.remove('tool-label-revealing')
      animation.cancel()
    }
    animation.onfinish = finish
    return finish
  }, [revealOnMount])

  if (!revealOnMount) return <span className={className}>{text}</span>

  return (
    <span ref={ref} className="relative inline-block min-w-0 max-w-full align-top">
      <span className={className}>{text}</span>
    </span>
  )
}
