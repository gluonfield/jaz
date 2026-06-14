import { FitAddon } from '@xterm/addon-fit'
import { Terminal as XTerm } from '@xterm/xterm'
import { Copy, Eraser, LoaderCircle, Power, RotateCw, Terminal, X } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { IconButton } from '@/components/ui/IconButton'
import { apiAuthenticatedWebSocketUrl } from '@/lib/api/client'
import type { Session } from '@/lib/api/types'

export const TERMINAL_PANEL_WIDTH = 640

type TerminalStatus = 'idle' | 'connecting' | 'connected' | 'stopping' | 'closed' | 'exited' | 'error'

interface ServerMessage {
  type: 'ready' | 'output' | 'exit' | 'error'
  data?: string
  cwd?: string
  code?: number
  error?: string
}

export function TerminalPanel({
  session,
  visible,
  onClose,
}: {
  session: Session
  visible: boolean
  onClose: () => void
}) {
  const cwd = session.runtime_ref?.cwd ?? ''
  const hostRef = useRef<HTMLDivElement | null>(null)
  const socketRef = useRef<WebSocket | null>(null)
  const termRef = useRef<XTerm | null>(null)
  const restartPendingRef = useRef(false)
  const [status, setStatus] = useState<TerminalStatus>('idle')
  const [remoteCwd, setRemoteCwd] = useState(cwd)
  const [error, setError] = useState('')
  const [exitCode, setExitCode] = useState<number | null>(null)
  const [connectNonce, setConnectNonce] = useState(0)

  useEffect(() => {
    setRemoteCwd(cwd)
    setError('')
    setExitCode(null)
  }, [cwd, session.id])

  useEffect(() => {
    if (!visible || !cwd || !hostRef.current) return
    let disposed = false
    const host = hostRef.current
    const term = new XTerm({
      cursorBlink: true,
      convertEol: false,
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
    termRef.current = term

    const params = new URLSearchParams({
      cols: String(term.cols),
      rows: String(term.rows),
    })
    const socket = new WebSocket(apiAuthenticatedWebSocketUrl(`/v1/sessions/${session.id}/terminal?${params}`))
    socketRef.current = socket
    setStatus('connecting')
    setError('')
    setExitCode(null)

    const send = (message: object) => {
      if (socket.readyState === WebSocket.OPEN) socket.send(JSON.stringify(message))
    }
    const fitNow = () => {
      if (disposed) return
      try {
        fit.fit()
        send({ type: 'resize', cols: term.cols, rows: term.rows })
      } catch {
        // xterm cannot measure while the panel is mid-collapse.
      }
    }
    const resizeObserver = new ResizeObserver(fitNow)
    resizeObserver.observe(host)
    const dataDisposable = term.onData((data) => send({ type: 'input', data }))
    const resizeDisposable = term.onResize(({ cols, rows }) => send({ type: 'resize', cols, rows }))

    socket.onopen = () => {
      if (disposed) return
      setStatus('connected')
      window.setTimeout(fitNow, 0)
    }
    socket.onmessage = (event) => {
      if (disposed || typeof event.data !== 'string') return
      const msg = parseServerMessage(event.data)
      if (!msg) return
      if (msg.type === 'ready') {
        setRemoteCwd(msg.cwd || cwd)
        setStatus('connected')
      } else if (msg.type === 'output') {
        term.write(msg.data ?? '')
      } else if (msg.type === 'exit') {
        setExitCode(msg.code ?? null)
        setStatus('exited')
        if (msg.error) setError(msg.error)
      } else if (msg.type === 'error') {
        setError(msg.error || 'Terminal error.')
        setStatus('error')
      }
    }
    socket.onerror = () => {
      if (disposed) return
      setError('Terminal connection failed.')
      setStatus('error')
    }
    socket.onclose = () => {
      if (disposed) return
      socketRef.current = null
      if (restartPendingRef.current) {
        restartPendingRef.current = false
        setConnectNonce((n) => n + 1)
        return
      }
      setStatus((current) => (current === 'exited' || current === 'error' ? current : 'closed'))
    }
    window.setTimeout(fitNow, 0)

    return () => {
      disposed = true
      resizeObserver.disconnect()
      dataDisposable.dispose()
      resizeDisposable.dispose()
      restartPendingRef.current = false
      socket.close()
      term.dispose()
      if (socketRef.current === socket) socketRef.current = null
      if (termRef.current === term) termRef.current = null
    }
  }, [connectNonce, cwd, session.id, visible])

  const sendControl = (type: 'terminate' | 'restart') => {
    const socket = socketRef.current
    if (socket?.readyState !== WebSocket.OPEN) return false
    try {
      socket.send(JSON.stringify({ type }))
      return true
    } catch {
      return false
    }
  }
  const restart = () => {
    termRef.current?.clear()
    setStatus('connecting')
    setError('')
    setExitCode(null)
    const socket = socketRef.current
    if (socket?.readyState === WebSocket.OPEN && sendControl('restart')) {
      restartPendingRef.current = true
    } else {
      restartPendingRef.current = false
      socket?.close()
      setConnectNonce((n) => n + 1)
    }
  }
  const terminate = () => {
    setStatus('stopping')
    if (!sendControl('terminate')) setStatus('closed')
  }
  const clear = () => termRef.current?.clear()
  const copySelection = () => {
    const selection = termRef.current?.getSelection()
    if (selection) void navigator.clipboard.writeText(selection)
  }

  return (
    <aside style={{ width: TERMINAL_PANEL_WIDTH }} className="flex h-full shrink-0 flex-col bg-bg p-2">
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-[14px] bg-surface shadow-[0_18px_46px_rgba(0,0,0,0.18)] ring-1 ring-border">
        <div className="flex h-11 shrink-0 items-center gap-1.5 border-b border-border px-2.5">
          <Terminal size={15} className="shrink-0 text-ink-3" aria-hidden />
          <span className="min-w-0 flex-1 truncate font-mono text-[12px] text-ink-2" title={remoteCwd || cwd}>
            {remoteCwd || cwd || 'No working directory'}
          </span>
          <TerminalStatusBadge status={status} exitCode={exitCode} />
          <IconButton size="sm" aria-label="Restart terminal" title="Restart terminal" onClick={restart} disabled={!cwd}>
            {status === 'connecting' ? <LoaderCircle size={14} className="animate-spin" /> : <RotateCw size={14} />}
          </IconButton>
          <IconButton size="sm" aria-label="Copy selection" title="Copy selection" onClick={copySelection} disabled={!cwd}>
            <Copy size={14} />
          </IconButton>
          <IconButton size="sm" aria-label="Clear terminal" title="Clear terminal" onClick={clear} disabled={!cwd}>
            <Eraser size={14} />
          </IconButton>
          <IconButton size="sm" variant="danger" aria-label="Terminate terminal" title="Terminate terminal" onClick={terminate} disabled={!cwd || status === 'exited'}>
            <Power size={14} />
          </IconButton>
          <IconButton size="sm" aria-label="Hide side panel" title="Hide side panel" onClick={onClose}>
            <X size={15} />
          </IconButton>
        </div>
        {error ? <p className="shrink-0 border-b border-border px-3 py-2 text-[12px] text-danger">{error}</p> : null}
        <div className="min-h-0 flex-1 bg-[#111318]">
          {cwd ? (
            <div ref={hostRef} className="jaz-terminal h-full w-full" />
          ) : (
            <div className="flex h-full items-center justify-center px-8 text-center text-[13px] text-ink-3">
              This session has no working directory.
            </div>
          )}
        </div>
      </div>
    </aside>
  )
}

function TerminalStatusBadge({ status, exitCode }: { status: TerminalStatus; exitCode: number | null }) {
  const label =
    status === 'connected'
      ? 'Live'
      : status === 'connecting'
        ? 'Connecting'
        : status === 'stopping'
          ? 'Stopping'
          : status === 'exited'
            ? exitCode === null
              ? 'Exited'
              : `Exit ${exitCode}`
            : status === 'error'
              ? 'Error'
              : status === 'closed'
                ? 'Closed'
                : 'Idle'
  return (
    <span className="shrink-0 rounded-full bg-bg px-2 py-1 font-mono text-[10px] text-ink-3 tabular-nums ring-1 ring-border/70">
      {label}
    </span>
  )
}

function parseServerMessage(data: string): ServerMessage | null {
  try {
    const parsed = JSON.parse(data) as ServerMessage
    return typeof parsed.type === 'string' ? parsed : null
  } catch {
    return null
  }
}

function terminalTheme() {
  const style = getComputedStyle(document.documentElement)
  const color = (name: string, fallback: string) => style.getPropertyValue(name).trim() || fallback
  return {
    background: '#111318',
    foreground: color('--color-ink', '#f0f1f4'),
    cursor: color('--color-primary', '#9aadff'),
    selectionBackground: color('--color-primary-soft', '#384767'),
    black: '#17191f',
    red: color('--color-danger', '#ff7a70'),
    green: color('--color-ok', '#77d694'),
    yellow: color('--color-running', '#e1c86a'),
    blue: color('--color-primary', '#9aadff'),
    magenta: '#d89cff',
    cyan: '#72d8e5',
    white: color('--color-ink-2', '#c9cbd3'),
    brightBlack: color('--color-ink-3', '#8b909e'),
    brightRed: color('--color-danger', '#ff8d82'),
    brightGreen: color('--color-ok', '#8ae5a5'),
    brightYellow: color('--color-running', '#edd779'),
    brightBlue: color('--color-primary-strong', '#b7c3ff'),
    brightMagenta: '#e6b5ff',
    brightCyan: '#93e6ef',
    brightWhite: color('--color-ink', '#f0f1f4'),
  }
}
