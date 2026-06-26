import { createFileRoute } from '@tanstack/react-router'
import { ArrowUp, LoaderCircle, Sparkles, X } from 'lucide-react'
import { type KeyboardEvent as ReactKeyboardEvent, type PointerEvent as ReactPointerEvent, useCallback, useEffect, useRef, useState } from 'react'
import { AgentModelControls, useNewThreadControls } from '@/components/session/useNewThreadControls'
import { dataURLToFile } from '@/components/ui/fileTransfer'
import { IconButton } from '@/components/ui/IconButton'
import { useToast } from '@/components/ui/toast'
import { createSession, uploadSessionAttachment } from '@/lib/api/sessions'
import { streamSessionMessage } from '@/lib/api/stream'
import { clientRuntime } from '@/lib/clientRuntime'
import { NEW_SESSION_DIRECTORY_KEY } from '@/lib/newSessionConfig'

export const Route = createFileRoute('/launcher')({
  component: LauncherPage,
})

interface Shot {
  id: string
  dataUrl: string
}

interface Selection {
  x: number
  y: number
  w: number
  h: number
}

const DRAG_THRESHOLD = 6

function LauncherPage() {
  const toast = useToast()
  const controls = useNewThreadControls()

  const [value, setValue] = useState('')
  const [shots, setShots] = useState<Shot[]>([])
  const [sending, setSending] = useState(false)
  const [capturing, setCapturing] = useState(false)
  const [selection, setSelection] = useState<Selection | null>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)

  const focusInput = () => inputRef.current?.focus()

  const reset = useCallback(() => {
    setValue('')
    setShots([])
    setSelection(null)
    if (inputRef.current) inputRef.current.style.height = 'auto'
  }, [])

  useEffect(() => {
    focusInput()
    return clientRuntime.onLauncherShown?.(() => {
      reset()
      focusInput()
    })
  }, [reset])

  const removeShot = (id: string) => setShots((prev) => prev.filter((shot) => shot.id !== id))

  const captureRect = async (rect: { x: number; y: number; width: number; height: number }) => {
    if (!clientRuntime.captureScreenRect) return
    setCapturing(true)
    try {
      const result = await clientRuntime.captureScreenRect(rect)
      if (result.denied) {
        toast('Allow Screen Recording for Jaz in System Settings, then reopen Jaz.', 'danger')
        return
      }
      if (!result.ok || !result.data) {
        toast("Couldn't capture that region. Try again.", 'danger')
        return
      }
      setShots((prev) => [...prev, { id: crypto.randomUUID(), dataUrl: `data:image/png;base64,${result.data}` }])
    } finally {
      setCapturing(false)
      focusInput()
    }
  }

  const onBackdropPointerDown = (event: ReactPointerEvent) => {
    if (event.button !== 0 || sending || capturing) return
    const sx = event.clientX
    const sy = event.clientY
    setSelection({ x: sx, y: sy, w: 0, h: 0 })
    const move = (e: PointerEvent) => {
      setSelection({
        x: Math.min(sx, e.clientX),
        y: Math.min(sy, e.clientY),
        w: Math.abs(e.clientX - sx),
        h: Math.abs(e.clientY - sy),
      })
    }
    const up = (e: PointerEvent) => {
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', up)
      setSelection(null)
      const w = Math.abs(e.clientX - sx)
      const h = Math.abs(e.clientY - sy)
      if (w < DRAG_THRESHOLD || h < DRAG_THRESHOLD) {
        clientRuntime.hideLauncher?.()
        return
      }
      void captureRect({ x: Math.min(sx, e.clientX), y: Math.min(sy, e.clientY), width: w, height: h })
    }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', up)
  }

  const submit = async () => {
    const text = value.trim()
    if (!text || sending) return
    if (!controls.runtimeAvailable) {
      toast('Connect an agent in Settings before starting a session.', 'danger')
      return
    }
    setSending(true)
    try {
      const directory = localStorage.getItem(NEW_SESSION_DIRECTORY_KEY) ?? ''
      const session = await createSession(controls.sessionConfig({ directory, worktree: false }, text))
      const uploaded = await Promise.all(
        shots.map(async (shot) =>
          uploadSessionAttachment(session.id, await dataURLToFile(shot.dataUrl, 'screenshot.png')),
        ),
      )
      void streamSessionMessage({
        sessionId: session.id,
        message: text,
        attachmentIds: uploaded.map((attachment) => attachment.id),
        signal: new AbortController().signal,
        onEvent: () => {},
      }).catch(() => {})
      clientRuntime.openInMain?.(`/sessions/${session.id}`)
      clientRuntime.hideLauncher?.()
      reset()
    } catch (error) {
      toast(`Couldn't start a session: ${(error as Error).message}`, 'danger')
    } finally {
      setSending(false)
    }
  }

  const onKeyDown = (event: ReactKeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === 'Escape') {
      event.preventDefault()
      clientRuntime.hideLauncher?.()
      return
    }
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault()
      void submit()
    }
  }

  return (
    <div className="relative h-full w-full">
      <div className="absolute inset-0 cursor-crosshair" onPointerDown={onBackdropPointerDown} />
      {selection ? (
        <div
          className="pointer-events-none absolute rounded-[3px] border-2 border-primary bg-primary/10"
          style={{ left: selection.x, top: selection.y, width: selection.w, height: selection.h }}
        />
      ) : null}

      <div className="absolute top-[16%] left-1/2 w-[720px] max-w-[calc(100vw-2rem)] -translate-x-1/2">
        <div className="overflow-hidden rounded-[18px] bg-surface shadow-[0_18px_50px_-12px_rgba(0,0,0,0.45)] ring-1 ring-border/60">
          <div className="flex items-center gap-3 px-4 py-3">
            <Sparkles size={20} className="shrink-0 text-primary" />
            <textarea
              ref={inputRef}
              rows={1}
              value={value}
              onChange={(event) => {
                setValue(event.target.value)
                autosize(event.currentTarget)
              }}
              onKeyDown={onKeyDown}
              placeholder="What can I help you with today?"
              className="max-h-32 flex-1 resize-none self-center bg-transparent text-[15px] leading-6 text-ink placeholder:text-ink-3 focus:outline-none"
            />
            <IconButton
              variant="primary"
              size="lg"
              aria-label="Send message"
              title="Send message"
              disabled={value.trim() === '' || sending}
              onClick={() => void submit()}
            >
              {sending ? <LoaderCircle size={16} className="animate-spin" /> : <ArrowUp size={18} />}
            </IconButton>
          </div>

          {shots.length > 0 ? (
            <div className="flex flex-wrap gap-2 px-4 pb-3">
              {shots.map((shot) => (
                <div key={shot.id} className="relative">
                  <img
                    src={shot.dataUrl}
                    alt="Captured screenshot"
                    className="h-14 w-20 rounded-lg object-cover ring-1 ring-border/60"
                  />
                  <button
                    type="button"
                    aria-label="Remove screenshot"
                    onClick={() => removeShot(shot.id)}
                    className="absolute -top-1.5 -right-1.5 grid size-5 cursor-pointer place-items-center rounded-full bg-ink text-bg shadow-sm transition-colors hover:bg-ink/80"
                  >
                    <X size={12} />
                  </button>
                </div>
              ))}
            </div>
          ) : null}

          <div className="flex items-center gap-1.5 border-t border-border/40 px-3 py-2">
            <AgentModelControls controls={controls} placement="below" disabled={sending} />
            <span className="ml-auto pr-1 text-[12px] text-ink-3">drag to screenshot · ↩ send · esc dismiss</span>
          </div>
        </div>
      </div>
    </div>
  )
}

function autosize(el: HTMLTextAreaElement): void {
  el.style.height = 'auto'
  el.style.height = `${Math.min(el.scrollHeight, 128)}px`
}
