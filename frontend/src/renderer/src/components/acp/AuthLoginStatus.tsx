import { AlertCircle, CheckCircle2, Copy, ExternalLink, LoaderCircle } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { useToast } from '@/components/ui/toast'
import { submitACPAuthLoginInput } from '@/lib/api/settings'
import type { ACPAuthLogin } from '@/lib/api/types'
import { writeClipboard } from '@/lib/clipboard'

export function AuthLoginStatus({
  job,
  running,
}: {
  job?: ACPAuthLogin
  running: boolean
}) {
  const toast = useToast()
  const opened = useRef('')
  const details = useMemo(() => authDetails(job), [job])
  const [code, setCode] = useState('')
  const [sending, setSending] = useState(false)
  const [sendError, setSendError] = useState('')

  // The remote/headless flow: the browser prints a code the CLI couldn't
  // capture, so the user hands it back here and the backend relays it to the
  // login process's stdin. The poll then reflects the result.
  const onSubmitCode = async () => {
    const value = code.trim()
    if (!job || !value) return
    setSending(true)
    setSendError('')
    try {
      await submitACPAuthLoginInput(job.id, value)
      setCode('')
    } catch (error) {
      setSendError(error instanceof Error ? error.message : 'Could not send the code')
    } finally {
      setSending(false)
    }
  }

  useEffect(() => {
    if (job?.agent !== 'codex' || job.status !== 'running' || !details.url) return
    const key = `${job.id}:${details.url}`
    if (opened.current === key) return
    opened.current = key
    openAuthURL(details.url)
  }, [details.url, job?.agent, job?.id, job?.status])

  if (!job && !running) return null

  const failed = job?.status === 'failed'
  const succeeded = job?.status === 'succeeded'
  const showAuthDetails = !failed && !succeeded && (details.url || details.code)
  const title = failed
    ? 'Sign-in failed'
    : succeeded
      ? 'Sign-in finished'
      : details.url || details.code
        ? 'Finish sign-in in your browser'
        : 'Starting sign-in...'
  const text = failed
    ? job?.error || 'The login command failed.'
    : succeeded
      ? 'Refreshing agent status.'
      : details.code
        ? 'Enter the one-time code on the auth page.'
        : details.url
          ? 'Approve the browser prompt to continue.'
          : 'Waiting for the agent CLI to print its browser login link.'

  return (
    <div
      className={`grid gap-2 rounded-[10px] px-3 py-2 text-[12px] ${
        failed ? 'bg-danger-soft text-danger' : 'bg-surface text-ink-2'
      }`}
    >
      <div className="flex min-w-0 items-start gap-2">
        <span className="mt-0.5 shrink-0">
          {failed ? (
            <AlertCircle size={14} />
          ) : succeeded ? (
            <CheckCircle2 size={14} className="text-primary" />
          ) : (
            <LoaderCircle size={14} className="animate-spin text-ink-3" />
          )}
        </span>
        <div className="min-w-0 flex-1">
          <p className="font-medium text-ink">{title}</p>
          <p className="mt-0.5 text-ink-3">{text}</p>
        </div>
      </div>

      {showAuthDetails ? (
        <div className="flex flex-wrap items-center gap-2">
          {details.url ? (
            <Button size="sm" variant="primary" onClick={() => openAuthURL(details.url)}>
              <ExternalLink size={13} />
              Open auth page
            </Button>
          ) : null}
          {details.code ? (
            <Button
              size="sm"
              aria-label="Copy auth code"
              title="Copy auth code"
              onClick={() => void copyAuthCode(details.code, toast)}
              className="bg-bg font-mono text-[12px] text-ink shadow-[inset_0_0_0_1px_color-mix(in_oklab,var(--color-border)_70%,transparent)] hover:bg-bg hover:text-ink"
            >
              <span className="tabular-nums">{details.code}</span>
              <Copy size={13} />
            </Button>
          ) : null}
        </div>
      ) : null}

      {job && showAuthDetails ? (
        <form
          onSubmit={(event) => {
            event.preventDefault()
            void onSubmitCode()
          }}
          className="grid gap-1.5"
        >
          <p className="text-ink-3">Did the browser show a code to paste back? Enter it here to finish.</p>
          <div className="flex items-center gap-1.5">
            <Input
              value={code}
              onChange={(event) => setCode(event.target.value)}
              placeholder="Paste the code from the browser"
              spellCheck={false}
              autoComplete="off"
              className="h-8 font-mono text-[12px]"
            />
            <Button size="sm" variant="primary" type="submit" disabled={sending || !code.trim()}>
              {sending ? <LoaderCircle size={13} className="animate-spin" /> : null}
              Submit
            </Button>
          </div>
          {sendError ? <p className="text-danger">{sendError}</p> : null}
        </form>
      ) : null}
    </div>
  )
}

function authDetails(job?: ACPAuthLogin): { url: string; code: string } {
  const output = stripANSI(job?.output ?? '')
  return {
    url: job?.auth_url || firstAuthURL(output),
    code: job?.auth_code || firstAuthCode(output),
  }
}

function stripANSI(value: string): string {
  return value.replace(new RegExp(`${String.fromCharCode(27)}\\[[0-9;]*[A-Za-z]`, 'g'), '')
}

function firstAuthURL(value: string): string {
  return value.match(/https:\/\/[^\s<>"']+/)?.[0]?.replace(/[.,)]$/g, '') ?? ''
}

function firstAuthCode(value: string): string {
  return value.match(/\b[A-Z0-9]{4}-[A-Z0-9]{4,6}\b/)?.[0] ?? ''
}

function openAuthURL(url: string): void {
  if (!/^https:\/\//i.test(url)) return
  window.open(url, '_blank', 'noopener,noreferrer')
}

async function copyAuthCode(code: string, toast: (message: string, tone?: 'ok' | 'danger') => void): Promise<void> {
  if (await writeClipboard(code)) {
    toast('Copied auth code')
  } else {
    toast("Couldn't copy auth code", 'danger')
  }
}
