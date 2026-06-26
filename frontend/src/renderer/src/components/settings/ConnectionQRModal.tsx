import { CheckCircle2, Clock3, Loader2, QrCode, TriangleAlert } from 'lucide-react'
import * as QRCode from 'qrcode'
import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Modal } from '@/components/ui/Modal'
import type { ConnectionQRStart, ConnectionQRStatus, IntegrationPlugin } from '@/lib/api/types'
import { PluginGlyph } from './ConnectionPluginVisuals'

export function ConnectionQRModal({
  plugin,
  qr,
  status,
  loading,
  onClose,
}: {
  plugin?: IntegrationPlugin
  qr?: ConnectionQRStart
  status?: ConnectionQRStatus
  loading: boolean
  onClose: () => void
}) {
  if (!plugin || !qr) return null
  const currentStatus = status?.status ?? qr.status
  const done = currentStatus === 'connected'
  const failed = currentStatus === 'expired' || currentStatus === 'failed'

  return (
    <Modal
      open
      onClose={onClose}
      title={`Connect ${plugin.name}`}
      description="Scan this code from the mobile app to link the account."
      icon={<PluginGlyph plugin={plugin} size={18} />}
      size="lg"
      footer={
        <>
          <QRStatusLine status={currentStatus} loading={loading} />
          <Button variant={done ? 'primary' : 'secondary'} onClick={onClose}>
            {done ? 'Done' : 'Close'}
          </Button>
        </>
      }
    >
      <div className="grid gap-4 sm:grid-cols-[210px_minmax(0,1fr)]">
        <QRCodeImage value={qr.code} failed={failed} />
        <div className="min-w-0 space-y-3">
          <div className="rounded-card bg-surface px-3 py-3">
            <p className="text-[12px] font-medium text-ink">Waiting for scan</p>
            <p className="mt-1 text-[12px] leading-5 text-ink-3">
              Keep this window open until the connection finishes. The code expires at{' '}
              {formatTime(qr.expires_at)}.
            </p>
          </div>
          {qr.instructions?.length ? (
            <ol className="space-y-1.5 text-[13px] leading-5 text-ink-2">
              {qr.instructions.map((instruction, index) => (
                <li key={instruction} className="flex gap-2">
                  <span className="mt-0.5 grid size-5 shrink-0 place-items-center rounded-full bg-surface-2 text-[11px] text-ink-3">
                    {index + 1}
                  </span>
                  <span>{instruction}</span>
                </li>
              ))}
            </ol>
          ) : null}
          {status?.error ? (
            <p className="rounded-card bg-danger-soft px-3 py-2 text-[12px] text-danger">
              {status.error}
            </p>
          ) : null}
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
      width: 210,
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
    <div className="relative grid size-[210px] shrink-0 place-items-center rounded-card bg-white p-3 shadow-[inset_0_0_0_1px_rgba(0,0,0,0.1)]">
      {src ? (
        <img src={src} alt="Connection QR code" className={failed ? 'size-full opacity-35' : 'size-full'} />
      ) : (
        <Loader2 size={18} className="animate-spin text-ink-3" />
      )}
      {failed ? (
        <div className="absolute inset-0 grid place-items-center rounded-card bg-white/70 text-danger">
          <TriangleAlert size={24} />
        </div>
      ) : null}
    </div>
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
  if (status === 'expired') {
    return (
      <span className="inline-flex items-center gap-1.5 text-[12px] text-danger">
        <Clock3 size={14} />
        Expired
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
      Waiting for scan
    </span>
  )
}

function formatTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}
