import { useLayoutEffect, useRef, useState, type ReactNode } from 'react'

export function FitContent({ children }: { children: ReactNode }) {
  const container = useRef<HTMLDivElement>(null)
  const content = useRef<HTMLDivElement>(null)
  const [scale, setScale] = useState(1)
  const [trim, setTrim] = useState(0)
  useLayoutEffect(() => {
    const box = container.current!
    const body = content.current!
    const measure = () => {
      if (box.clientWidth && box.clientHeight) {
        const width = Math.min(1, box.clientWidth / body.scrollWidth)
        setScale(Math.min(width, box.clientHeight / body.scrollHeight))
        // A scaled body keeps its unscaled height in layout; the margin gives
        // that back so a content-sized item ends where its content does.
        setTrim(body.scrollHeight * (width - 1))
      }
    }
    const observer = new ResizeObserver(measure)
    observer.observe(box)
    observer.observe(body)
    measure()
    return () => observer.disconnect()
  }, [children])
  return <div ref={container} className="canvas-fit">
    <div ref={content} className="canvas-fit-content" style={{ transform: `scale(${scale})`, marginBottom: trim }}>{children}</div>
  </div>
}
