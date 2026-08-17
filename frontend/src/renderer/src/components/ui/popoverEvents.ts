export function bindPopoverEvents({
  anchor,
  menu,
  followAnchor,
  onAnchorMove,
  onClose,
}: {
  anchor: () => Element | null
  menu: () => Element | null
  followAnchor: boolean
  onAnchorMove: () => void
  onClose: () => void
}): () => void {
  let frame = 0
  const scheduleAnchorMove = () => {
    window.cancelAnimationFrame(frame)
    frame = window.requestAnimationFrame(onAnchorMove)
  }
  const onDown = (event: MouseEvent) => {
    const target = event.target
    if (target instanceof Node && (anchor()?.contains(target) || menu()?.contains(target))) return
    onClose()
  }
  const onKey = (event: KeyboardEvent) => {
    if (event.key !== 'Escape') return
    event.stopPropagation()
    onClose()
  }
  const onScroll = (event: Event) => {
    if (event.target instanceof Node && menu()?.contains(event.target)) return
    if (followAnchor) scheduleAnchorMove()
    else onClose()
  }
  const onResize = followAnchor ? scheduleAnchorMove : onClose
  document.addEventListener('mousedown', onDown)
  document.addEventListener('keydown', onKey)
  window.addEventListener('scroll', onScroll, true)
  window.addEventListener('resize', onResize)
  return () => {
    window.cancelAnimationFrame(frame)
    document.removeEventListener('mousedown', onDown)
    document.removeEventListener('keydown', onKey)
    window.removeEventListener('scroll', onScroll, true)
    window.removeEventListener('resize', onResize)
  }
}
