import { FitAddon } from '@xterm/addon-fit'
import { Terminal } from '@xterm/xterm'
import { apiWebSocket } from '@/lib/api/client'

type TerminalEvent =
  | { type: 'ready'; cwd?: string }
  | { type: 'exit'; code?: number; error?: string }
  | { type: 'error'; error: string }
  | { type: 'closed' }

type TerminalCommand = 'terminate' | 'restart'

type ClientMessage =
  | { type: TerminalCommand }
  | { type: 'input'; data: string }
  | { type: 'resize'; cols: number; rows: number }

export function connectTerminal(
  host: HTMLElement,
  path: string,
  onEvent: (event: TerminalEvent) => void,
) {
  const term = new Terminal({
    cursorBlink: true,
    fontFamily: "'JetBrains Mono Variable', ui-monospace, 'SF Mono', monospace",
    fontSize: 12,
    lineHeight: 1.25,
    macOptionIsMeta: true,
    scrollback: 5000,
    theme: terminalTheme(),
  })
  const fit = new FitAddon()
  term.loadAddon(fit)
  term.open(host)
  term.focus()

  const measure = () => {
    if (!host.clientWidth || !host.clientHeight) return
    try {
      fit.fit()
    } catch {
      // xterm cannot measure while a panel is mid-collapse.
    }
  }
  measure()
  const size = () => ({ cols: term.cols, rows: term.rows })
  const params = new URLSearchParams({ cols: String(term.cols), rows: String(term.rows) })
  let socket: WebSocket
  try {
    socket = apiWebSocket(`${path}?${params}`)
  } catch {
    term.dispose()
    onEvent({ type: 'error', error: 'Terminal connection failed.' })
    return
  }
  const send = (message: ClientMessage) => {
    if (socket.readyState !== WebSocket.OPEN) return false
    try {
      socket.send(JSON.stringify(message))
      return true
    } catch {
      return false
    }
  }
  const observer = new ResizeObserver(measure)
  observer.observe(host)
  const input = term.onData((data) => send({ type: 'input', data }))
  const resize = term.onResize(() => send({ type: 'resize', ...size() }))

  socket.onopen = () => send({ type: 'resize', ...size() })
  socket.onmessage = ({ data }) => {
    if (typeof data !== 'string') return
    const event = parseTerminalMessage(data)
    if (!event) return
    if (event.type === 'output') term.write(event.data ?? '')
    else onEvent(event)
  }
  socket.onerror = () => onEvent({ type: 'error', error: 'Terminal connection failed.' })
  socket.onclose = () => onEvent({ type: 'closed' })

  return {
    term,
    send: (type: TerminalCommand) => send({ type }),
    close: () => {
      observer.disconnect()
      input.dispose()
      resize.dispose()
      socket.onclose = null
      socket.onmessage = null
      socket.onopen = null
      socket.onerror = null
      socket.close()
      term.dispose()
    },
  }
}

function parseTerminalMessage(data: string): Exclude<TerminalEvent, { type: 'closed' }> | { type: 'output'; data?: string } | null {
  try {
    const message = JSON.parse(data)
    switch (message?.type) {
      case 'ready': return { type: 'ready', cwd: message.cwd }
      case 'output': return { type: 'output', data: message.data }
      case 'exit': return { type: 'exit', code: message.code, error: message.error }
      case 'error': return { type: 'error', error: message.error || 'Terminal error.' }
      default: return null
    }
  } catch {
    return null
  }
}

// The terminal surface is always dark, so its palette is fixed light-on-dark —
// deriving it from the app's theme tokens turns text dark-on-dark in light mode.
function terminalTheme() {
  return {
    background: '#111318',
    foreground: '#f0f1f4',
    cursor: '#9aadff',
    selectionBackground: '#384767',
    black: '#17191f',
    red: '#ff7a70',
    green: '#77d694',
    yellow: '#e1c86a',
    blue: '#9aadff',
    magenta: '#d89cff',
    cyan: '#72d8e5',
    white: '#c9cbd3',
    brightBlack: '#8b909e',
    brightRed: '#ff8d82',
    brightGreen: '#8ae5a5',
    brightYellow: '#edd779',
    brightBlue: '#b7c3ff',
    brightMagenta: '#e6b5ff',
    brightCyan: '#93e6ef',
    brightWhite: '#f0f1f4',
  }
}
