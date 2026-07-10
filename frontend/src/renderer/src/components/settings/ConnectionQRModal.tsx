import { CheckCircle2, Clock3, KeyRound, Loader2, QrCode, RefreshCw, TriangleAlert } from 'lucide-react'
import * as QRCode from 'qrcode'
import { type FormEvent, useEffect, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Modal } from '@/components/ui/Modal'
import type { ConnectionQRStart, ConnectionQRStatus, IntegrationPlugin } from '@/lib/api/types'
import { PluginGlyph, PluginIcon } from './ConnectionPluginVisuals'

export function ConnectionQRModal({
  plugin,
  qr,
  status,
  loading,
  refreshing,
  passwordSubmitting,
  onClose,
  onRefresh,
  onSubmitPassword,
}: {
  plugin?: IntegrationPlugin
  qr?: ConnectionQRStart
  status?: ConnectionQRStatus
  loading: boolean
  refreshing: boolean
  passwordSubmitting: boolean
  onClose: () => void
  onRefresh: () => void
  onSubmitPassword: (password: string) => void
}) {
  if (!plugin || !qr) return null
  const currentStatus = status?.status ?? qr.status
  const currentCode = status?.code || qr.code
  const expiresAt = status?.expires_at || qr.expires_at
  const accountID = status?.account_id
  const done = currentStatus === 'connected'
  const failed = currentStatus === 'expired' || currentStatus === 'failed'
  const passwordRequired = currentStatus === 'password_required'

  return (
    <Modal
      open
      onClose={onClose}
      title={`Connect ${plugin.name}`}
      description={`Scan with ${plugin.name} on your phone.`}
      icon={<PluginGlyph plugin={plugin} size={18} />}
      size="lg"
      footer={
        <>
          <QRStatusLine status={currentStatus} loading={loading} />
          <div className="flex items-center gap-2">
            {failed ? (
              <Button variant="secondary" onClick={onRefresh} disabled={refreshing}>
                <RefreshCw size={14} className={refreshing ? 'animate-spin' : undefined} />
                {refreshing ? 'Getting code' : 'New QR code'}
              </Button>
            ) : null}
            <Button variant={done ? 'primary' : 'secondary'} onClick={onClose}>
              {done ? 'Done' : 'Close'}
            </Button>
          </div>
        </>
      }
    >
      <div className="grid gap-5 sm:grid-cols-[248px_minmax(0,1fr)]">
        <div className="mx-auto w-full max-w-[248px] space-y-3 sm:max-w-none">
          <div className="rounded-[20px] bg-surface p-3 shadow-[inset_0_0_0_1px_rgba(0,0,0,0.04)]">
            <QRCodeImage value={currentCode} failed={failed} />
          </div>
          <div className="flex items-center justify-center gap-2 text-[12px] text-ink-3">
            <Clock3 size={13} />
            <span className="tabular-nums">Expires {formatTime(expiresAt)}</span>
          </div>
        </div>
        <div className="min-w-0 space-y-4">
          <StatusCard plugin={plugin} status={currentStatus} error={status?.error} accountID={accountID} />
          {passwordRequired ? (
            <PasswordCard
              provider={plugin.name}
              error={status?.error}
              submitting={passwordSubmitting}
              onSubmitPassword={onSubmitPassword}
            />
          ) : null}
          <StepList instructions={qr.instructions} />
        </div>
      </div>
    </Modal>
  )
}

function QRCodeImage({ value, failed }: { value: string; failed: boolean }) {
  const [src, setSrc] = useState('')

  useEffect(() => {
    let cancelled = false
    setSrc('')
    void QRCode.toDataURL(value, {
      errorCorrectionLevel: 'M',
      margin: 1,
      width: 224,
      color: { dark: '#111111', light: '#ffffff' },
    })
      .then((next) => {
        if (!cancelled) setSrc(next)
      })
      .catch(() => {
        if (!cancelled) setSrc('')
      })
    return () => {
      cancelled = true
    }
  }, [value])

  return (
    <div className="relative grid aspect-square w-full shrink-0 place-items-center rounded-[14px] bg-white p-3 shadow-[inset_0_0_0_1px_rgba(0,0,0,0.1)]">
      {src ? (
        <img src={src} alt="Connection QR code" className={failed ? 'size-full opacity-35' : 'size-full'} />
      ) : (
        <Loader2 size={18} className="animate-spin text-ink-3" />
      )}
      {failed ? (
        <div className="absolute inset-0 grid place-items-center rounded-[14px] bg-white/75 text-danger">
          <div className="grid size-12 place-items-center rounded-full bg-danger-soft">
            <TriangleAlert size={24} />
          </div>
        </div>
      ) : null}
    </div>
  )
}

function StatusCard({
  plugin,
  status,
  error,
  accountID,
}: {
  plugin: IntegrationPlugin
  status: string
  error?: string
  accountID?: string
}) {
  const content = statusContent(plugin.name, status, error, accountID)
  return (
    <div className="rounded-card bg-surface px-3 py-3 shadow-[inset_0_0_0_1px_rgba(0,0,0,0.04)]">
      <div className="flex items-start gap-3">
        <PluginIcon plugin={plugin} compact />
        <div className="min-w-0">
          <p className={`text-[13px] font-medium ${content.tone}`}>{content.title}</p>
          <p className="mt-1 text-[12px] leading-5 text-ink-3">{content.detail}</p>
        </div>
      </div>
    </div>
  )
}

function StepList({ instructions }: { instructions: string[] }) {
  return (
    <ol className="space-y-2 text-[13px] leading-5 text-ink-2">
      {instructions.map((instruction, index) => (
        <li key={`${index}-${instruction}`} className="flex gap-2.5">
          <span className="mt-0.5 grid size-6 shrink-0 place-items-center rounded-full bg-surface text-[11px] font-medium tabular-nums text-ink-3">
            {index + 1}
          </span>
          <span className="pt-0.5">{instruction}</span>
        </li>
      ))}
    </ol>
  )
}

function PasswordCard({
  provider,
  error,
  submitting,
  onSubmitPassword,
}: {
  provider: string
  error?: string
  submitting: boolean
  onSubmitPassword: (password: string) => void
}) {
  const [password, setPassword] = useState('')

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!password || submitting) return
    onSubmitPassword(password)
  }

  return (
    <form
      className="rounded-card bg-surface px-3 py-3 shadow-[inset_0_0_0_1px_rgba(0,0,0,0.04)]"
      onSubmit={submit}
    >
      <label className="block text-[12px] font-medium text-ink" htmlFor="connection-qr-password">
        {provider} password
      </label>
      <div className="mt-2 flex items-center gap-2">
        <input
          id="connection-qr-password"
          type="password"
          value={password}
          autoComplete="current-password"
          placeholder="Two-step verification password"
          className="h-9 min-w-0 flex-1 rounded-lg border border-border bg-white px-3 text-[13px] text-ink outline-none transition-colors placeholder:text-ink-3 focus:border-primary"
          onChange={(event) => setPassword(event.target.value)}
        />
        <Button type="submit" variant="primary" disabled={!password || submitting}>
          {submitting ? <Loader2 size={14} className="animate-spin" /> : <KeyRound size={14} />}
          {submitting ? 'Checking' : 'Continue'}
        </Button>
      </div>
      {error ? <p className="mt-2 text-[12px] leading-5 text-danger">{error}</p> : null}
    </form>
  )
}

function QRStatusLine({ status, loading }: { status: string; loading: boolean }) {
  if (status === 'connected') {
    return (
      <span className="inline-flex items-center gap-1.5 text-[12px] text-ok">
        <CheckCircle2 size={14} />
        Connected
      </span>
    )
  }
  if (status === 'scanned') {
    return (
      <span className="inline-flex items-center gap-1.5 text-[12px] text-primary">
        <CheckCircle2 size={14} />
        Scanned
      </span>
    )
  }
  if (status === 'expired') {
    return (
      <span className="inline-flex items-center gap-1.5 text-[12px] text-danger">
        <Clock3 size={14} />
        Expired
      </span>
    )
  }
  if (status === 'password_required') {
    return (
      <span className="inline-flex items-center gap-1.5 text-[12px] text-primary">
        <KeyRound size={14} />
        Password needed
      </span>
    )
  }
  if (status === 'failed') {
    return (
      <span className="inline-flex items-center gap-1.5 text-[12px] text-danger">
        <TriangleAlert size={14} />
        Failed
      </span>
    )
  }
  return (
    <span className="inline-flex items-center gap-1.5 text-[12px] text-ink-3">
      {loading ? <Loader2 size={14} className="animate-spin" /> : <QrCode size={14} />}
      Waiting for QR scan
    </span>
  )
}

function statusContent(provider: string, status: string, error?: string, accountID?: string) {
  if (status === 'connected') {
    return {
      title: 'Connected',
      detail: accountID ? `${accountID} is ready in Jaz.` : `${provider} is ready in Jaz.`,
      tone: 'text-ok',
    }
  }
  if (status === 'scanned') {
    return {
      title: 'Scanned',
      detail: error || 'Finish any confirmation on your phone. This window will update automatically.',
      tone: 'text-primary',
    }
  }
  if (status === 'expired') {
    return {
      title: 'QR code expired',
      detail: 'Get a new code and scan it from your phone.',
      tone: 'text-danger',
    }
  }
  if (status === 'password_required') {
    return {
      title: 'Password needed',
      detail: `Enter your ${provider} two-step verification password to finish sign-in.`,
      tone: 'text-primary',
    }
  }
  if (status === 'failed') {
    return {
      title: 'QR sign-in failed',
      detail: error || 'Get a new code and try again.',
      tone: 'text-danger',
    }
  }
  return {
    title: 'Scan the QR code',
    detail: error || 'Keep this window open. Jaz will connect as soon as your phone approves the scan.',
    tone: 'text-ink',
  }
}

function formatTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}
