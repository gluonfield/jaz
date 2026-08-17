const listeners = new Map()
let frame
let anchorMoves = 0
let closed = 0
globalThis.Node = class {}

globalThis.document = {
  addEventListener: (name, listener) => listeners.set(`document:${name}`, listener),
  removeEventListener: (name) => listeners.delete(`document:${name}`),
}
globalThis.window = {
  addEventListener: (name, listener) => listeners.set(`window:${name}`, listener),
  removeEventListener: (name) => listeners.delete(`window:${name}`),
  cancelAnimationFrame: () => {},
  requestAnimationFrame: (callback) => {
    frame = callback
    return 1
  },
}

const { bindPopoverEvents } = await import('./popoverEvents')
const cleanup = bindPopoverEvents({
  anchor: () => ({ contains: () => false }),
  menu: () => ({ contains: () => false }),
  followAnchor: true,
  onAnchorMove: () => {
    anchorMoves += 1
  },
  onClose: () => {
    closed += 1
  },
})
listeners.get('window:scroll')({ target: new globalThis.Node() })
const openAfterScroll = closed === 0
frame()
const followedScroll = anchorMoves === 1
cleanup()

globalThis.postMessage({ openAfterScroll, followedScroll })
