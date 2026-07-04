// The Appearance font size is applied as CSS `zoom` on <html> (see lib/appearance).
// getBoundingClientRect() and window.inner{Width,Height} report in screen px, but a
// position:fixed element portalled under <html> has its own left/top/right/bottom
// re-multiplied by that zoom — so feeding screen px straight back double-scales it.
// These helpers convert screen px to the layout px that fixed offsets live in, so a
// portalled panel lands on its anchor at any font size. All are no-ops at 1x.

function rootZoom(): number {
  const zoom = parseFloat(getComputedStyle(document.documentElement).zoom)
  return zoom > 0 ? zoom : 1
}

export function toLayoutRect(rect: DOMRect): DOMRect {
  const zoom = rootZoom()
  if (zoom === 1) return rect
  return new DOMRect(rect.x / zoom, rect.y / zoom, rect.width / zoom, rect.height / zoom)
}

export function layoutRect(el: Element): DOMRect {
  return toLayoutRect(el.getBoundingClientRect())
}

// A viewport point (e.g. a pointer's clientX/clientY) in layout px.
export function layoutPoint(x: number, y: number): { x: number; y: number } {
  const zoom = rootZoom()
  return zoom === 1 ? { x, y } : { x: x / zoom, y: y / zoom }
}

export function layoutViewport(): { width: number; height: number } {
  const zoom = rootZoom()
  return { width: window.innerWidth / zoom, height: window.innerHeight / zoom }
}
