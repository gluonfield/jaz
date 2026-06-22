import { LoaderCircle } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { type FormEvent, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { PixelField } from '@/components/ui/PixelField'
import { apiBaseUrl } from '@/lib/api/client'
import {
  cancelPendingApproval,
  connectionPreference,
  connectRemote,
  rememberedRemoteUrl,
  startLocal,
  useConnection,
} from '@/lib/connection'
import { localDeviceLabel } from '@/lib/deviceLabel'
import { useTheme } from '@/lib/theme'

const EASE = [0.22, 1, 0.36, 1] as const

const stagger = {
  hidden: {},
  show: { transition: { staggerChildren: 0.08, delayChildren: 0.12 } },
}

const rise = {
  hidden: { opacity: 0, y: 14, filter: 'blur(6px)' },
  show: { opacity: 1, y: 0, filter: 'blur(0px)', transition: { duration: 0.5, ease: EASE } },
}

const swap = {
  initial: { opacity: 0, y: 8 },
  animate: { opacity: 1, y: 0 },
  exit: { opacity: 0, y: -8 },
  transition: { duration: 0.2, ease: 'easeOut' as const },
}

// Floats over the live app while the health poll retries a lost backend; the
// window only falls back to the launch screen after the reconnect grace.
export function ReconnectingBanner({ show }: { show: boolean }) {
  return (
    <div className="pointer-events-none fixed inset-x-0 top-[60px] z-50 flex justify-center">
      <AnimatePresence>
        {show && (
          <motion.div
            initial={{ opacity: 0, y: -8 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -8 }}
            transition={{ duration: 0.2, ease: 'easeOut' }}
            className="flex items-center gap-2 rounded-full bg-surface-2 px-3 py-1.5 text-[12px] text-ink-2 ring-1 ring-border"
          >
            <LoaderCircle size={12} className="animate-spin" />
            Reconnecting to backend…
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  )
}

// Full-window gate shown whenever no backend is reachable: first launch,
// failed startup probe, or a lost connection mid-session. The particle field
// renders the wordmark, so the chrome stays text-light. First launch is a
// welcome — two ways to run jaz — not an error.
export function LaunchScreen() {
  const { status, error, pairing } = useConnection()
  // PixelField samples the palette at mount; remount it when the theme flips.
  const { resolved } = useTheme()
  const [mode, setMode] = useState<'options' | 'remote'>('options')
  const [busy, setBusy] = useState<'local' | 'remote' | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)
  const deviceLabel = localDeviceLabel()
  // last remote wins; otherwise the active URL (the local default until a
  // remote was ever used) seeds the field as an editable starting point
  const [url, setUrl] = useState(() => rememberedRemoteUrl() || apiBaseUrl())

  const onStartLocal = async () => {
    setBusy('local')
    setActionError(null)
    const err = await startLocal()
    // on success the connection store flips to connected and this unmounts
    if (err) {
      setActionError(err)
      setBusy(null)
    }
  }

  const onConnect = async (e: FormEvent) => {
    e.preventDefault()
    setBusy('remote')
    setActionError(null)
    const err = await connectRemote(url)
    if (err) {
      setActionError(err)
      setBusy(null)
    }
  }

  // The launch screen flashes for a sub-second on every boot while the first
  // probe runs; only spin up the GPU field once we know we're staying.
  const showField = status === 'disconnected' || status === 'pending_approval'
  // While 'checking' we know what we're waiting on — tailor the copy so a
  // remembered server or a local start reads as intentional, not a hang.
  const checkingCopy =
    connectionPreference()?.mode === 'remote'
      ? 'Connecting to your server…'
      : connectionPreference()?.mode === 'local'
        ? `Starting jaz on ${deviceLabel}…`
        : 'Connecting to backend…'

  return (
    <div className="relative flex h-full flex-col bg-bg">
      {showField && <PixelField key={resolved} calm={mode === 'remote' || busy !== null} />}
      <div className="titlebar-drag relative h-[52px] shrink-0" />
      {/* offset the titlebar so the content is optically centered */}
      <div className="relative flex flex-1 flex-col items-center justify-center px-6 pb-[52px]">
        <AnimatePresence mode="wait">
          {status === 'checking' ? (
            <motion.div
              key="checking"
              initial={{ opacity: 0 }}
              animate={{ opacity: 1, transition: { duration: 0.3, delay: 0.2 } }}
              exit={{ opacity: 0, transition: { duration: 0.15 } }}
              className="flex flex-col items-center gap-3"
            >
              <span className="size-2 animate-pulse rounded-full bg-primary" />
              <p className="text-[13px] text-ink-3">{checkingCopy}</p>
            </motion.div>
          ) : status === 'pending_approval' && pairing ? (
            <motion.div
              key="pending"
              variants={stagger}
              initial="hidden"
              animate="show"
              className="flex w-full max-w-[340px] flex-col items-center"
            >
              <motion.div variants={rise} className="mb-4 flex size-12 items-center justify-center rounded-full bg-primary-soft">
                <span className="size-2.5 animate-pulse rounded-full bg-primary" />
              </motion.div>
              <motion.h1
                variants={rise}
                className="text-balance text-center text-[22px] font-semibold tracking-tight text-ink"
              >
                Waiting for approval
              </motion.h1>
              <motion.p variants={rise} className="mt-2 text-pretty text-center text-[13px] text-ink-2">
                Approve this device from Settings on a connected Jaz app.
              </motion.p>
              <motion.div
                variants={rise}
                className="mt-5 w-full rounded-[16px] bg-surface/90 p-3 text-left shadow-[0_8px_30px_rgba(0,0,0,0.10)] backdrop-blur-[2px]"
              >
                <p className="text-[12px] font-medium text-ink">{pairing.deviceName}</p>
                <p className="mt-1 truncate font-mono text-[11px] text-ink-3">{pairing.url}</p>
                <p className="mt-2 text-[11px] text-ink-3">Request expires {formatApprovalExpiry(pairing.expiresAt)}.</p>
              </motion.div>
              <motion.div variants={rise} className="mt-4">
                <Button variant="ghost" onClick={cancelPendingApproval}>
                  Cancel
                </Button>
              </motion.div>
            </motion.div>
          ) : (
            <motion.div
              key="options"
              variants={stagger}
              initial="hidden"
              animate="show"
              className="flex w-full max-w-[320px] flex-col items-center"
            >
              <motion.h1
                variants={rise}
                className="text-balance text-center text-[22px] font-semibold tracking-tight text-ink"
              >
                {error ? 'Reconnect to jaz' : 'Welcome to jaz'}
              </motion.h1>

              {error && (
                <motion.p
                  variants={rise}
                  className="mt-2 text-pretty text-center text-[13px] text-ink-2"
                >
                  The backend jaz was using is unreachable. Start one here or point jaz at another
                  server.
                </motion.p>
              )}

              <motion.div variants={rise} className="mt-6 w-full">
                <AnimatePresence mode="wait" initial={false}>
                  {mode === 'options' ? (
                    <motion.div key="opts" {...swap} className="flex flex-col gap-2">
                      <ChoiceButton
                        primary
                        label={`Run on ${deviceLabel}`}
                        busyLabel="Starting backend…"
                        busy={busy === 'local'}
                        disabled={busy !== null}
                        onClick={onStartLocal}
                      />
                      <ChoiceButton
                        label="Connect to a server"
                        disabled={busy !== null}
                        onClick={() => setMode('remote')}
                      />
                    </motion.div>
                  ) : (
                    <motion.form
                      key="remote"
                      {...swap}
                      onSubmit={onConnect}
                      className="flex w-full flex-col gap-2.5 rounded-[16px] bg-surface/90 p-3 shadow-[0_8px_30px_rgba(0,0,0,0.10)] backdrop-blur-[2px]"
                    >
                      <div className="px-0.5">
                        <p className="text-[13px] font-medium text-ink">Connect to a server</p>
                        <p className="mt-0.5 text-[12px] text-ink-3">Paste the client URL your server printed.</p>
                      </div>
                      <Input
                        autoFocus
                        value={url}
                        onChange={(e) => setUrl(e.target.value)}
                        placeholder="http://192.168.1.10:5299?key=…"
                        spellCheck={false}
                        className="rounded-full font-mono text-[12px]"
                      />
                      <div className="flex items-center justify-end gap-2">
                        <Button
                          variant="ghost"
                          disabled={busy !== null}
                          onClick={() => {
                            setMode('options')
                            setActionError(null)
                          }}
                        >
                          Back
                        </Button>
                        <Button variant="primary" type="submit" disabled={busy !== null || !url.trim()}>
                          {busy === 'remote' && <LoaderCircle size={14} className="animate-spin" />}
                          {busy === 'remote' ? 'Connecting…' : 'Connect'}
                        </Button>
                      </div>
                    </motion.form>
                  )}
                </AnimatePresence>
              </motion.div>

              <AnimatePresence>
                {actionError && (
                  <motion.p
                    initial={{ opacity: 0, y: 4 }}
                    animate={{ opacity: 1, y: 0 }}
                    exit={{ opacity: 0 }}
                    className="mt-4 max-w-[320px] text-pretty text-center text-[12px] text-danger"
                  >
                    {actionError}
                  </motion.p>
                )}
              </AnimatePresence>
            </motion.div>
          )}
        </AnimatePresence>
      </div>
    </div>
  )
}

// Full-width rounded pill: the welcome's two ways to run jaz. No icons — the
// label carries it. Primary = the cobalt recommended path (run locally),
// secondary = a quiet surface pill (connect to a server). Soft drop shadow
// floats it over the particle field; scales on press.
function ChoiceButton({
  label,
  busyLabel,
  busy = false,
  primary = false,
  disabled,
  onClick,
}: {
  label: string
  busyLabel?: string
  busy?: boolean
  primary?: boolean
  disabled?: boolean
  onClick: () => void
}) {
  return (
    <motion.button
      type="button"
      disabled={disabled}
      onClick={onClick}
      whileTap={disabled ? undefined : { scale: 0.97 }}
      className={`flex h-11 w-full cursor-pointer items-center justify-center gap-2 rounded-full text-[14px] font-medium transition-colors duration-150 disabled:cursor-default disabled:opacity-60 ${
        primary
          ? 'bg-primary text-on-primary shadow-[0_8px_24px_rgba(0,0,0,0.14)] enabled:hover:bg-primary-strong'
          : 'bg-surface/90 text-ink shadow-[0_6px_20px_rgba(0,0,0,0.08)] backdrop-blur-[2px] enabled:hover:bg-surface'
      }`}
    >
      {busy ? <LoaderCircle size={15} className="animate-spin" /> : null}
      {busy && busyLabel ? busyLabel : label}
    </motion.button>
  )
}

function formatApprovalExpiry(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return 'soon'
  return date.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
}
