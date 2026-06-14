import { AlertCircle, CheckCircle2, ExternalLink, LoaderCircle } from 'lucide-react'
import { useEffect, useMemo, useRef } from 'react'
import { Button } from '@/components/ui/Button'
import type { ACPAuthLogin } from '@/lib/api/types'

export function AuthLoginStatus({
  job,
  running,
}: {
  job?: ACPAuthLogin
  running: boolean
}) {
  const opened = useRef('')
  const details = useMemo(() => authDetails(job), [job])

  useEffect(() => {
    if (job?.agent !== 'codex' || !details.url) return
    const key = `${job.id}:${details.url}`
    if (opened.current === key) return
    opened.current = key
    openAuthURL(details.url)
  }, [details.url, job?.agent, job?.id])

  if (!job && !running) return null

  const failed = job?.status === 'failed'
  const succeeded = job?.status === 'succeeded'
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

      {details.url || details.code ? (
        <div className="flex flex-wrap items-center gap-2">
          {details.url ? (
            <Button size="sm" variant="primary" onClick={() => openAuthURL(details.url)}>
              <ExternalLink size={13} />
              Open auth page
            </Button>
          ) : null}
          {details.code ? (
            <span className="inline-flex h-7 items-center rounded-full bg-bg px-2.5 font-mono text-[12px] text-ink shadow-[inset_0_0_0_1px_color-mix(in_oklab,var(--color-border)_70%,transparent)]">
              {details.code}
            </span>
          ) : null}
        </div>
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
