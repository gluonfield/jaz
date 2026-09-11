type Point = { x: number; y: number }

export class BrowserCursor {
  private point: Point = { x: 24, y: 32 }
  private frame = 0
  private timeout: ReturnType<typeof setTimeout> | undefined
  private reject: ((error: Error) => void) | undefined
  private readonly arrow: HTMLDivElement

  constructor(private readonly layer: HTMLElement) {
    this.arrow = document.createElement('div')
    this.arrow.dataset.browserAgentCursor = ''
    this.arrow.style.cssText = 'position:absolute;left:0;top:0;width:28px;height:32px;transform-origin:2px 2px;opacity:0;filter:drop-shadow(0 2px 3px #0005)'
    this.arrow.innerHTML = '<svg width="28" height="32" viewBox="0 0 28 32" aria-hidden="true"><path d="M2 2L24 17L14 19L9 29Z" fill="#262626" stroke="white" stroke-width="2" stroke-linejoin="round"/></svg>'
    layer.append(this.arrow)
  }

  move(to: Point, dragging = false): Promise<void> {
    this.cancel()
    const from = this.point
    const distance = Math.hypot(to.x - from.x, to.y - from.y)
    const bend = distance > 196 && !dragging ? Math.min(distance * 0.16, 100) : 0
    const control = {
      x: Math.max(0, Math.min(this.layer.clientWidth, (from.x + to.x) / 2 - (to.y - from.y) / Math.max(1, distance) * bend)),
      y: Math.max(0, Math.min(this.layer.clientHeight, (from.y + to.y) / 2 + (to.x - from.x) / Math.max(1, distance) * bend)),
    }
    this.arrow.style.opacity = '1'
    if (document.hidden || this.layer.closest('[inert]') || window.matchMedia('(prefers-reduced-motion: reduce)').matches || distance < 0.5) {
      this.draw(to, 0, 1)
      return Promise.resolve()
    }
    return new Promise((resolve, reject) => {
      this.reject = reject
      const started = performance.now()
      const finish = () => {
        cancelAnimationFrame(this.frame)
        clearTimeout(this.timeout)
        this.reject = undefined
        this.draw(to, 0, 1)
        this.think()
        resolve()
      }
      const tick = (now: number) => {
        const seconds = (now - started) / 1000
        const rate = dragging ? 65 : distance <= 196 ? 20 : 13
        const progress = 1 - (1 + rate * seconds) * Math.exp(-rate * seconds)
        const rest = 1 - progress
        const pulse = Math.sin(progress * Math.PI)
        this.draw({
          x: rest * rest * from.x + 2 * rest * progress * control.x + progress * progress * to.x,
          y: rest * rest * from.y + 2 * rest * progress * control.y + progress * progress * to.y,
        }, Math.sign(to.x - from.x) * 12.5 * pulse, 1 - 0.18 * pulse)
        if (progress >= 0.999) {
          finish()
        } else {
          this.frame = requestAnimationFrame(tick)
        }
      }
      this.timeout = setTimeout(finish, 1500)
      this.frame = requestAnimationFrame(tick)
    })
  }

  press(pressed: boolean): void {
    this.arrow.style.scale = pressed ? '0.96' : '1'
  }

  destroy(): void {
    this.cancel()
    this.arrow.remove()
  }

  private draw(point: Point, rotation: number, stretch: number): void {
    this.point = point
    this.arrow.style.transform = `translate3d(${point.x - 2}px,${point.y - 2}px,0) rotate(${rotation}deg) scale(${stretch},1)`
  }

  private think(): void {
    const started = performance.now()
    const tick = (now: number) => {
      const elapsed = (now - started) / 1000
      const progress = Math.min(1, elapsed / 1.41)
      this.draw(this.point, Math.sin(elapsed / 0.66 * Math.PI * 2) * Math.sin(progress * Math.PI) * 12.5, 1)
      if (progress < 1) {
        this.frame = requestAnimationFrame(tick)
      }
    }
    this.frame = requestAnimationFrame(tick)
  }

  cancel(): void {
    cancelAnimationFrame(this.frame)
    clearTimeout(this.timeout)
    this.reject?.(new Error('Browser cursor movement was cancelled'))
    this.reject = undefined
    this.arrow.style.scale = '1'
  }
}
