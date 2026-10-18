import { type RefObject, useLayoutEffect, useState } from 'react'

let canvas: CanvasRenderingContext2D | null = null

// Whether a messenger composer's draft fits on one line in the middle column
// of its row, between the attach button and the controls. The column keeps
// that width when a wrapped draft spans the row above the buttons, so the
// answer never flips back and forth between the two layouts.
export function useFitsOneRow(rowRef: RefObject<HTMLElement | null>, field: RefObject<HTMLTextAreaElement | null>, text: string): boolean {
  const [fits, setFits] = useState(true)
  useLayoutEffect(() => {
    const row = rowRef.current
    const textarea = field.current
    if (!row || !textarea) return
    const measure = () => setFits(fitsColumn(row, textarea, text))
    measure()
    const observer = new ResizeObserver(measure)
    for (const element of [row, ...row.children]) observer.observe(element)
    return () => observer.disconnect()
  }, [rowRef, field, text])
  return fits
}

function fitsColumn(row: HTMLElement, textarea: HTMLTextAreaElement, text: string): boolean {
  if (text.includes('\n')) return false
  const style = getComputedStyle(textarea)
  const column = parseFloat(getComputedStyle(row).gridTemplateColumns.split(' ')[1])
  canvas ??= document.createElement('canvas').getContext('2d')!
  canvas.font = style.font
  canvas.letterSpacing = style.letterSpacing
  return canvas.measureText(text).width <= column - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight)
}
