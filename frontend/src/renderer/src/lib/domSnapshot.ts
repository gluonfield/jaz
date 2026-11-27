// Paints what the window currently shows onto a canvas by walking the DOM:
// backgrounds, borders, text, inline SVG and same-origin images. It is a likeness
// for effects that take the page apart, not a faithful render: shadows,
// gradients and pseudo-elements are left out.

const TRANSPARENT = /^(transparent|rgba\(0, 0, 0, 0\))$/

type Painter = {
  context: CanvasRenderingContext2D
  icons: Map<Element, HTMLImageElement>
  skip: Element | null
}

function roundedPath(context: CanvasRenderingContext2D, rect: DOMRect, radius: number) {
  context.beginPath()
  context.roundRect(rect.left, rect.top, rect.width, rect.height, Math.min(radius, rect.width / 2, rect.height / 2))
}

function loadIcon(svg: SVGSVGElement): Promise<[Element, HTMLImageElement] | null> {
  const rect = svg.getBoundingClientRect()
  if (rect.width === 0 || rect.bottom < 0 || rect.top > window.innerHeight) return Promise.resolve(null)
  const copy = svg.cloneNode(true) as SVGSVGElement
  copy.setAttribute('xmlns', 'http://www.w3.org/2000/svg')
  copy.setAttribute('width', String(rect.width))
  copy.setAttribute('height', String(rect.height))
  copy.style.color = getComputedStyle(svg).color
  const image = new Image()
  image.src = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(new XMLSerializer().serializeToString(copy))}`
  return image.decode().then(
    () => [svg, image],
    () => null,
  )
}

function sameOrigin(source: string): boolean {
  return /^(data|blob):/.test(source) || new URL(source, location.href).origin === location.origin
}

function paintBorders(context: CanvasRenderingContext2D, rect: DOMRect, style: CSSStyleDeclaration) {
  const sides = [
    ['Top', rect.left, rect.top, rect.width, 0],
    ['Bottom', rect.left, rect.bottom, rect.width, 0],
    ['Left', rect.left, rect.top, 0, rect.height],
    ['Right', rect.right, rect.top, 0, rect.height],
  ] as const
  for (const [side, x, y, width, height] of sides) {
    const size = parseFloat(style[`border${side}Width`])
    const color = style[`border${side}Color`]
    if (!size || style[`border${side}Style`] === 'none' || TRANSPARENT.test(color)) continue
    context.fillStyle = color
    const inward = side === 'Bottom' || side === 'Right' ? -size : 0
    context.fillRect(x + (width ? 0 : inward), y + (height ? 0 : inward), width || size, height || size)
  }
}

function paintText(context: CanvasRenderingContext2D, text: Text, style: CSSStyleDeclaration) {
  context.font = style.font
  context.fillStyle = style.color
  context.letterSpacing = style.letterSpacing === 'normal' ? '0px' : style.letterSpacing
  context.textBaseline = 'middle'
  const upper = style.textTransform === 'uppercase'
  const range = document.createRange()
  for (const word of text.data.matchAll(/\S+/g)) {
    range.setStart(text, word.index)
    range.setEnd(text, word.index + word[0].length)
    const rect = range.getBoundingClientRect()
    if (rect.width === 0 || rect.bottom < 0 || rect.top > window.innerHeight) continue
    context.fillText(upper ? word[0].toUpperCase() : word[0], rect.left, rect.top + rect.height / 2)
  }
}

function paintElement(painter: Painter, element: Element, inherited: number) {
  if (element === painter.skip) return
  const { context } = painter
  const style = getComputedStyle(element)
  const opacity = inherited * parseFloat(style.opacity)
  if (style.display === 'none' || opacity < 0.01) return
  const rect = element.getBoundingClientRect()
  const clips = style.overflowX !== 'visible' || style.overflowY !== 'visible'
  const offscreen = rect.bottom < 0 || rect.top > window.innerHeight || rect.right < 0 || rect.left > window.innerWidth
  if (clips && (offscreen || rect.width === 0 || rect.height === 0)) return
  const hidden = style.visibility === 'hidden'
  const radius = parseFloat(style.borderTopLeftRadius)
  context.globalAlpha = opacity

  if (!hidden && !offscreen) {
    if (!TRANSPARENT.test(style.backgroundColor)) {
      context.fillStyle = style.backgroundColor
      roundedPath(context, rect, radius)
      context.fill()
    }
    paintBorders(context, rect, style)
    const icon = painter.icons.get(element)
    if (icon) context.drawImage(icon, rect.left, rect.top, rect.width, rect.height)
    if (element instanceof HTMLImageElement && element.complete && element.naturalWidth > 0 && sameOrigin(element.currentSrc)) {
      context.save()
      roundedPath(context, rect, radius)
      context.clip()
      context.drawImage(element, rect.left, rect.top, rect.width, rect.height)
      context.restore()
    }
    if (element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement) {
      context.font = style.font
      context.fillStyle = style.color
      context.textBaseline = 'middle'
      context.globalAlpha = opacity * (element.value ? 1 : 0.5)
      const left = rect.left + parseFloat(style.paddingLeft) + parseFloat(style.borderLeftWidth)
      context.fillText(element.type === 'password' ? '•'.repeat(element.value.length) : element.value || element.placeholder, left, rect.top + rect.height / 2)
    }
  }
  if (element instanceof SVGElement) return

  if (clips) {
    context.save()
    roundedPath(context, rect, radius)
    context.clip()
  }
  for (const child of element.childNodes) {
    if (child instanceof Element) {
      paintElement(painter, child, opacity)
    } else if (child instanceof Text && !hidden) {
      context.globalAlpha = opacity
      paintText(context, child, style)
    }
  }
  if (clips) context.restore()
}

// `skip` and everything inside it is left out, so an overlay can snapshot the page beneath itself.
export async function snapshotWindow(scale: number, skip: Element | null): Promise<HTMLCanvasElement> {
  const canvas = document.createElement('canvas')
  canvas.width = Math.round(window.innerWidth * scale)
  canvas.height = Math.round(window.innerHeight * scale)
  const context = canvas.getContext('2d')!
  const loaded = await Promise.all([...document.querySelectorAll('svg')].map(loadIcon))
  context.scale(scale, scale)
  context.fillStyle = getComputedStyle(document.documentElement).getPropertyValue('--color-bg')
  context.fillRect(0, 0, window.innerWidth, window.innerHeight)
  paintElement({ context, icons: new Map(loaded.filter((entry) => entry !== null)), skip }, document.body, 1)
  return canvas
}
