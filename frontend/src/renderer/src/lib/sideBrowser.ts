import { BrowserCursor } from '@/lib/browserCursor'
import { BROWSER_CDP_METHODS, type BrowserCommand, type BrowserCommandRequest } from '@shared/browserControl'
import { BrowserRepl, type BrowserAction, type BrowserActionResult } from '@/lib/browserRepl'
import { previewDisplayUrl, resolvePreviewSource } from '@/lib/api/preview'

export type BrowserViewport = {
  getWebContentsId(): number
  getURL(): string
  getTitle(): string
}

export class SideBrowser {
  private viewport: BrowserViewport | null = null
  private cursor: BrowserCursor | null = null
  private waiting: { resolve: () => void; reject: (error: Error) => void } | null = null
  private generation = 0
  private readonly repl: BrowserRepl

  constructor(
    private readonly open: (url: string) => void,
    private readonly command: (request: BrowserCommandRequest) => Promise<unknown>,
    action: (input: BrowserAction, signal: AbortSignal) => Promise<BrowserActionResult>,
  ) {
    this.repl = new BrowserRepl((input, signal) => input.action === 'cdp'
      ? this.sendCDP(input)
      : action(input, signal))
  }

  attach(viewport: BrowserViewport, layer: HTMLElement): () => void {
    this.viewport = viewport
    this.cursor = new BrowserCursor(layer)
    this.waiting?.resolve()
    return () => {
      this.cancel()
      this.cursor?.destroy()
      this.cursor = null
      this.viewport = null
    }
  }

  async call({ method, params }: BrowserCommand): Promise<unknown> {
    const generation = this.generation
    const viewport = this.viewport
    if (method === 'Jaz.run') {
      return this.repl.run(String(params?.code ?? ''))
    }
    if (method === 'Jaz.open') {
      if (!viewport) {
        const ready = new Promise<void>((resolve, reject) => {
          this.waiting = { resolve, reject }
        })
        const timeout = setTimeout(() => this.waiting?.reject(new Error('Side browser did not open')), 15_000)
        this.open(String(params?.url))
        try {
          await ready
        } finally {
          clearTimeout(timeout)
          this.waiting = null
        }
        return {}
      }
      const requested = String(params?.url)
      const current = viewport.getURL()
      if (current === requested || previewDisplayUrl(current) === requested) {
        return {}
      }
      method = 'Jaz.navigate'
      params = { url: await resolvePreviewSource(requested) }
    }
    if (method === 'Jaz.tab') {
      return viewport ? { id: String(viewport.getWebContentsId()), url: viewport.getURL(), title: viewport.getTitle(), active: true } : {}
    }
    if (!viewport) {
      throw new Error('Side browser is closed; call browser_navigate to open it')
    }
    if (method === 'Input.dispatchMouseEvent' && params) {
      const x = Number(params.x)
      const y = Number(params.y)
      if (!Number.isFinite(x) || !Number.isFinite(y)) {
        throw new Error('Mouse coordinates must be finite')
      }
      if (params.type === 'mouseMoved' || params.type === 'mouseWheel') {
        const scale = Number(await this.command({ webContentsId: viewport.getWebContentsId(), method: 'Jaz.cursorScale' }))
        if (this.viewport !== viewport || generation !== this.generation) {
          throw new Error('Side browser changed during the action')
        }
        await this.cursor?.move({ x: x * scale, y: y * scale }, params.buttons === 1)
      }
      this.cursor?.press(params.type === 'mousePressed' || params.buttons === 1)
    }
    if (this.viewport !== viewport || generation !== this.generation) {
      throw new Error('Side browser changed during the action')
    }
    return this.command({ webContentsId: viewport.getWebContentsId(), method, params })
  }

  private async sendCDP(command: BrowserCommand): Promise<BrowserActionResult> {
    if (!BROWSER_CDP_METHODS.has(command.method)) {
      throw new Error(`Unsupported tab CDP command: ${command.method}`)
    }
    if (!this.viewport) {
      throw new Error('Side browser is closed; call tab.goto to open it')
    }
    const data = await this.command({ ...command, webContentsId: this.viewport.getWebContentsId() })
    return { status: 'ok', data }
  }

  dispose(): void {
    this.cancel()
    this.cursor?.destroy()
    this.cursor = null
    this.viewport = null
  }

  cancel(): void {
    this.generation += 1
    this.waiting?.reject(new Error('Side browser disconnected'))
    this.cursor?.cancel()
    this.repl.cancel()
  }
}
