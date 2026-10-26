import { BrowserWindow, ipcMain, type WebContents } from 'electron'
import { join } from 'node:path'
import { CLAUDE_VOICE_ARGUMENT, CLAUDE_VOICE_CHANNEL, type ClaudeVoiceCommand, type ClaudeVoiceReply } from '@shared/claudeVoice'
import { PREVIEW_PARTITION } from '@shared/preview'
import { trustedWindow } from '@main/permissions'

export class ClaudeVoiceBridge {
  readonly window: BrowserWindow
  private loaded: Promise<void>
  private sequence = 0
  private pending = new Map<number, { resolve: (value: ClaudeVoiceReply['value']) => void; reject: (error: Error) => void; timer: ReturnType<typeof setTimeout> }>()

  constructor(owner: BrowserWindow, onEvent: (id: string, data: string) => void, options = { origin: 'https://claude.ai', preload: join(__dirname, '../preload/index.js') }) {
    const { origin, preload } = options
    this.window = new BrowserWindow({
      parent: owner,
      show: false,
      focusable: false,
      skipTaskbar: true,
      webPreferences: {
        preload,
        partition: PREVIEW_PARTITION,
        contextIsolation: true,
        sandbox: true,
        nodeIntegration: false,
        backgroundThrottling: false,
        additionalArguments: [CLAUDE_VOICE_ARGUMENT],
      },
    })
    this.window.webContents.setWindowOpenHandler(() => ({ action: 'deny' }))
    this.window.webContents.on('will-navigate', (event, url) => {
      if (new URL(url).origin !== origin) {
        event.preventDefault()
      }
    })
    const fromBridge = (event: Electron.IpcMainEvent) => event.sender === this.window.webContents && event.senderFrame === event.sender.mainFrame && new URL(event.senderFrame.url).origin === origin
    const reply = (event: Electron.IpcMainEvent, result: ClaudeVoiceReply) => {
      if (!fromBridge(event)) {
        return
      }
      const pending = this.pending.get(result.requestId)
      if (!pending) {
        return
      }
      this.pending.delete(result.requestId)
      clearTimeout(pending.timer)
      if (result.error) {
        pending.reject(new Error(result.error))
      } else {
        pending.resolve(result.value)
      }
    }
    const receive = (event: Electron.IpcMainEvent, id: string, data: string) => {
      if (fromBridge(event) && typeof data === 'string' && data.length <= 524288) {
        onEvent(id, data)
      }
    }
    ipcMain.on(CLAUDE_VOICE_CHANNEL + ':reply', reply)
    ipcMain.on(CLAUDE_VOICE_CHANNEL + ':event', receive)
    const dispose = () => this.dispose()
    owner.webContents.once('destroyed', dispose)
    owner.webContents.once('render-process-gone', dispose)
    const navigate = ({ isMainFrame, isSameDocument }: Electron.Event<Electron.WebContentsDidStartNavigationEventParams>) => {
      if (isMainFrame && !isSameDocument) {
        this.dispose()
      }
    }
    owner.webContents.on('did-start-navigation', navigate)
    this.window.once('closed', () => {
      owner.webContents.removeListener('destroyed', dispose)
      owner.webContents.removeListener('render-process-gone', dispose)
      owner.webContents.removeListener('did-start-navigation', navigate)
      ipcMain.removeListener(CLAUDE_VOICE_CHANNEL + ':reply', reply)
      ipcMain.removeListener(CLAUDE_VOICE_CHANNEL + ':event', receive)
      for (const pending of this.pending.values()) {
        clearTimeout(pending.timer)
        pending.reject(new Error('Claude voice connection closed.'))
      }
      this.pending.clear()
    })
    this.loaded = new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        reject(new Error('Loading Claude voice timed out.'))
        this.dispose()
      }, 20000)
      void this.window.loadURL(origin + '/new').then(resolve, reject).finally(() => clearTimeout(timer))
    })
  }

  async request(command: ClaudeVoiceCommand): Promise<ClaudeVoiceReply['value']> {
    await this.loaded
    if (this.window.isDestroyed()) {
      throw new Error('Claude voice connection closed.')
    }
    const requestId = ++this.sequence
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(requestId)
        reject(new Error('Claude voice request timed out.'))
      }, command.type === 'status' ? 10000 : 35000)
      this.pending.set(requestId, { resolve, reject, timer })
      this.window.webContents.send(CLAUDE_VOICE_CHANNEL + ':request', requestId, command)
    })
  }

  dispose(): void {
    if (!this.window.isDestroyed()) {
      this.window.destroy()
    }
  }
}

export function installClaudeVoice(options?: { origin: string; preload: string }): void {
  const bridges = new Map<number, { bridge: ClaudeVoiceBridge; calls: Set<string>; requests: number }>()
  ipcMain.handle(CLAUDE_VOICE_CHANNEL, async (event, command: ClaudeVoiceCommand) => {
    const owner = trustedWindow(event)
    if (!command || !['status', 'start', 'send', 'result', 'close'].includes(command.type) || JSON.stringify(command).length > 262144) {
      throw new Error('Invalid Claude voice command.')
    }
    if (command.type !== 'status' && (typeof command.id !== 'string' || !/^[a-zA-Z0-9-]{1,128}$/.test(command.id))) {
      throw new Error('Invalid Claude voice call.')
    }
    if (command.type === 'start') {
      const options = command.options
      if (!options || typeof options.sdp !== 'string' || !options.sdp.startsWith('v=0') || options.sdp.length > 65536 ||
        typeof options.context !== 'string' || options.context.length > 131072 ||
        typeof options.voice !== 'string' || options.voice.length > 128) {
        throw new Error('Invalid Claude voice connection.')
      }
    }
    if (command.type === 'send' && typeof command.data !== 'string') {
      throw new Error('Invalid Claude voice message.')
    }
    if (command.type === 'result' && (typeof command.toolUseId !== 'string' || typeof command.text !== 'string' || typeof command.isError !== 'boolean')) {
      throw new Error('Invalid Claude voice result.')
    }
    let entry = bridges.get(owner)
    if (command.type !== 'status' && command.type !== 'start' && (!entry || entry.bridge.window.isDestroyed() || !entry.calls.has(command.id))) {
      if (command.type === 'close') {
        return
      }
      throw new Error('Claude voice is disconnected.')
    }
    if (!entry || entry.bridge.window.isDestroyed()) {
      const parent = BrowserWindow.fromWebContents(event.sender)
      if (!parent) {
        throw new Error('Claude voice requires a Jaz window.')
      }
      const calls = new Set<string>()
      const sender: WebContents = event.sender
      const bridge = new ClaudeVoiceBridge(parent, (id, data) => {
        if (calls.has(id) && !sender.isDestroyed()) {
          sender.send(CLAUDE_VOICE_CHANNEL + ':event', id, data)
        }
      }, options)
      entry = { bridge, calls, requests: 0 }
      bridges.set(owner, entry)
      bridge.window.once('closed', () => {
        if (!sender.isDestroyed()) {
          for (const id of calls) {
            sender.send(CLAUDE_VOICE_CHANNEL + ':event', id, JSON.stringify({ type: 'jaz_disconnected' }))
          }
        }
        if (bridges.get(owner)?.bridge === bridge) {
          bridges.delete(owner)
        }
      })
    }
    if (command.type === 'start') {
      entry.calls.add(command.id)
    }
    entry.requests++
    try {
      return await entry.bridge.request(command)
    } catch (error) {
      if (command.type === 'start') {
        entry.calls.delete(command.id)
      }
      throw error
    } finally {
      if (command.type === 'close') {
        entry.calls.delete(command.id)
      }
      entry.requests--
      if (!entry.calls.size && !entry.requests) {
        entry.bridge.dispose()
      }
    }
  })
}
