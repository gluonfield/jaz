export const BROWSER_IDLE_MS = 5 * 60_000

export class BrowserRetention {
  private timer: ReturnType<typeof setTimeout> | undefined
  private check: AbortController | undefined
  private pending = 0
  private eligible = false

  constructor(
    private readonly canRelease: (signal: AbortSignal) => Promise<boolean>,
    private readonly release: () => void,
    private readonly idleMs = BROWSER_IDLE_MS,
  ) {}

  configure(eligible: boolean): void {
    this.eligible = eligible
    this.reset()
  }

  async run<T>(action: () => Promise<T>): Promise<T> {
    this.pending += 1
    this.reset()
    try {
      return await action()
    } finally {
      this.pending -= 1
      this.reset()
    }
  }

  private reset(): void {
    clearTimeout(this.timer)
    this.check?.abort()
    if (!this.eligible || this.pending) return
    this.timer = setTimeout(async () => {
      const check = new AbortController()
      this.check = check
      let release = false
      try {
        release = await this.canRelease(check.signal)
      } catch {
        // An unavailable agent status cannot establish that its browser is idle.
      }
      if (check.signal.aborted) return
      if (release) {
        this.eligible = false
        this.release()
      } else {
        this.reset()
      }
    }, this.idleMs)
  }
}
