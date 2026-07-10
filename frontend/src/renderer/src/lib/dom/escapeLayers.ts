let nextLayerID = 0
const layers: string[] = []

export function createEscapeLayerID(prefix: string): string {
  nextLayerID += 1
  return `${prefix}-${nextLayerID}`
}

export function pushEscapeLayer(id: string): () => void {
  layers.push(id)
  return () => removeEscapeLayer(id)
}

export function isTopEscapeLayer(id: string): boolean {
  return layers[layers.length - 1] === id
}

export function consumeEscapeKey(event: KeyboardEvent) {
  event.preventDefault()
  event.stopPropagation()
  event.stopImmediatePropagation()
}

function removeEscapeLayer(id: string) {
  const index = layers.lastIndexOf(id)
  if (index >= 0) layers.splice(index, 1)
}
