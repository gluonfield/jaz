import type { WebContents } from 'electron'

type FrameTree = {
  frame: { id: string; parentId?: string; loaderId: string; url: string }
  childFrames?: FrameTree[]
}

export class BrowserFrames {
  private readonly sessions = new Map<string, string>()

  constructor(private readonly target: WebContents) {
    target.debugger.on('message', (_event, method, params, sessionId) => {
      if (method === 'Target.attachedToTarget' && params.targetInfo.type === 'iframe') {
        this.sessions.set(params.sessionId, sessionId)
      }
      if (method === 'Target.detachedFromTarget') {
        const removed = new Set<string>([params.sessionId])
        for (const parent of removed) {
          this.sessions.delete(parent)
          for (const [child, owner] of this.sessions) {
            if (owner === parent) {
              removed.add(child)
            }
          }
        }
      }
    })
    target.debugger.on('detach', () => this.sessions.clear())
  }

  owns(sessionId: string): boolean {
    return this.sessions.has(sessionId)
  }

  async list(): Promise<Array<{ sessionId: string; frameTree: FrameTree }>> {
    const result: Array<{ sessionId: string; frameTree: FrameTree }> = []
    const pending = new Set([''])
    for (const sessionId of pending) {
      if (sessionId && !this.sessions.has(sessionId)) {
        continue
      }
      await this.target.debugger.sendCommand('Target.setAutoAttach', {
        autoAttach: true,
        waitForDebuggerOnStart: false,
        flatten: true,
        filter: [{ type: 'iframe' }, { exclude: true }],
      }, sessionId || undefined)
      const { frameTree } = await this.target.debugger.sendCommand('Page.getFrameTree', {}, sessionId || undefined)
      result.push({ sessionId, frameTree })
      for (const child of this.sessions.keys()) {
        pending.add(child)
      }
    }
    return result
  }
}
