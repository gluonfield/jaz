import { Copy, Eraser, LoaderCircle, Power, RotateCw, Terminal } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { IconButton } from '@/components/ui/IconButton'
import type { Session } from '@/lib/api/types'
import { connectTerminal } from '@/lib/terminalConnection'

type TerminalStatus = 'idle' | 'connecting' | 'connected' | 'stopping' | 'closed' | 'exited' | 'error'

export function TerminalPanel({
  session,
  visible,
}: {
  session: Session
  visible: boolean
}) {
  const cwd = session.runtime_ref?.cwd ?? ''
  const hostRef = useRef<HTMLDivElement | null>(null)
  const connectionRef = useRef<ReturnType<typeof connectTerminal>>(undefined)
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
    if (!cwd || !hostRef.current) return
    setStatus('connecting')
    setError('')
    setExitCode(null)
    const connection = connectTerminal(hostRef.current, `/v1/sessions/${session.id}/terminal`, (event) => {
      switch (event.type) {
        case 'ready':
          setRemoteCwd(event.cwd || cwd)
          setStatus('connected')
          break
        case 'exit':
          setExitCode(event.code ?? null)
          setStatus('exited')
          if (event.error) setError(event.error)
          break
        case 'error':
          setError(event.error)
          setStatus('error')
          break
        case 'closed':
          if (restartPendingRef.current) {
            restartPendingRef.current = false
            setConnectNonce((n) => n + 1)
            return
          }
          setStatus((current) => (current === 'exited' || current === 'error' ? current : 'closed'))
          break
      }
    })
    connectionRef.current = connection

    return () => {
      restartPendingRef.current = false
      connection?.close()
      connectionRef.current = undefined
    }
  }, [connectNonce, cwd, session.id])

  useEffect(() => {
    if (visible) connectionRef.current?.term.focus()
  }, [visible])

  const restart = () => {
    connectionRef.current?.term.clear()
    setStatus('connecting')
    setError('')
    setExitCode(null)
    if (connectionRef.current?.send('restart')) {
      restartPendingRef.current = true
    } else {
      restartPendingRef.current = false
      setConnectNonce((n) => n + 1)
    }
  }
  const terminate = () => {
    setStatus('stopping')
    if (!connectionRef.current?.send('terminate')) setStatus('closed')
  }
  const clear = () => connectionRef.current?.term.clear()
  const copySelection = () => {
    const selection = connectionRef.current?.term.getSelection()
    if (selection) void navigator.clipboard.writeText(selection)
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
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
