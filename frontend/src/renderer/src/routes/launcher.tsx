import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { ArrowUp, LoaderCircle, Monitor, Sparkles, X } from 'lucide-react'
import { type KeyboardEvent as ReactKeyboardEvent, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { dataURLToFile } from '@/components/ui/fileTransfer'
import { IconButton } from '@/components/ui/IconButton'
import { useToast } from '@/components/ui/toast'
import { agentLabel } from '@/lib/agentLabel'
import { enabledACPAgents } from '@/lib/agentRuntimes'
import { createSession, uploadSessionAttachment } from '@/lib/api/sessions'
import { agentSettingsQuery } from '@/lib/api/settings'
import { streamSessionMessage } from '@/lib/api/stream'
import { clientRuntime } from '@/lib/clientRuntime'
import { createSessionInput, launcherAgent, NEW_SESSION_DIRECTORY_KEY } from '@/lib/newSessionConfig'

export const Route = createFileRoute('/launcher')({
  component: LauncherPage,
})

interface Shot {
  id: string
  // The capture arrives as base64; a data URL displays directly and becomes a
  // File only at upload — no object-URL lifecycle to manage.
  dataUrl: string
}

// A floating, frameless quick-launcher (⌥Space from any app): draft a message,
// optionally drag a screen region in as an attachment, then hand the new
// session to the main window. The launcher acts as a full client — it creates
// the session, uploads attachments, and fires the turn (which runs detached
// server-side); the main window streams the reply through the session's own
// live subscription.
function LauncherPage() {
  const toast = useToast()
  const settingsQuery = useQuery(agentSettingsQuery)
  const agentSettings = settingsQuery.data
  const runtimeAvailable = settingsQuery.isSuccess && enabledACPAgents(agentSettings).length > 0
  const agent = useMemo(() => launcherAgent(agentSettings), [agentSettings])

  const [value, setValue] = useState('')
  const [shots, setShots] = useState<Shot[]>([])
  const [sending, setSending] = useState(false)
  const [capturing, setCapturing] = useState(false)
  const inputRef = useRef<HTMLTextAreaElement>(null)

  const focusInput = () => inputRef.current?.focus()

  const reset = useCallback(() => {
    setValue('')
    setShots([])
    if (inputRef.current) inputRef.current.style.height = 'auto'
  }, [])

  // Each summon starts clean and focused.
  useEffect(() => {
    focusInput()
    return clientRuntime.onLauncherShown?.(() => {
      reset()
      focusInput()
    })
  }, [reset])

  const removeShot = (id: string) => setShots((prev) => prev.filter((shot) => shot.id !== id))

  const capture = async () => {
    if (capturing || !clientRuntime.captureScreenRegion) return
    setCapturing(true)
    try {
      const result = await clientRuntime.captureScreenRegion()
      if (!result.ok || !result.data) return // cancelled, or capture denied
      setShots((prev) => [
        ...prev,
        { id: crypto.randomUUID(), dataUrl: `data:image/png;base64,${result.data}` },
      ])
    } finally {
      setCapturing(false)
      focusInput()
    }
  }

  const submit = async () => {
    const text = value.trim()
    if (!text || sending) return
    if (!runtimeAvailable || agent === '') {
      toast('Connect an agent in Settings before starting a session.', 'danger')
      return
    }
    setSending(true)
    try {
      const directory = localStorage.getItem(NEW_SESSION_DIRECTORY_KEY) ?? ''
      const session = await createSession(
        createSessionInput(agentSettings, { agent, directory, worktree: false }, text),
      )
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
    <div className="flex h-full w-full items-start justify-center p-3">
      <div className="w-full overflow-hidden rounded-[20px] bg-surface shadow-[0_24px_60px_rgba(0,0,0,0.38)] ring-1 ring-border/60">
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
          <span className="shrink-0 text-[13px] text-ink-3">{runtimeAvailable ? agentLabel(agent) : ''}</span>
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

        <div className="flex items-center gap-2 border-t border-border/40 px-3 py-2">
          <button
            type="button"
            onClick={() => void capture()}
            disabled={capturing}
            className="flex cursor-pointer items-center gap-1.5 rounded-full px-2 py-1 text-[13px] text-ink-2 transition-colors hover:bg-surface-2 disabled:cursor-default disabled:opacity-50"
          >
            <Monitor size={14} />
            {capturing ? 'Select a region…' : 'Drag to take a screenshot'}
          </button>
          <span className="ml-auto pr-1 text-[12px] text-ink-3">↩ send · esc dismiss</span>
        </div>
      </div>
    </div>
  )
}

function autosize(el: HTMLTextAreaElement): void {
  el.style.height = 'auto'
  el.style.height = `${Math.min(el.scrollHeight, 128)}px`
}
