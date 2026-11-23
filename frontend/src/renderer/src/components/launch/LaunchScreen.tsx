import { Cloud, Laptop, LoaderCircle, X } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { useState } from 'react'
import { RemoteServerForm } from '@/components/connection/RemoteServerForm'
import { DitherTerrain, DitherWordmark } from '@/components/launch/DitherArt'
import { useKnownBackends } from '@/lib/backends'
import { clientRuntime } from '@/lib/clientRuntime'
import {
  connectionPreference,
  connectRemote,
  currentConnection,
  type ConnectionStatus,
  forgetBackend,
  startCloudSignIn,
  startLocal,
  useConnection,
} from '@/lib/connection'
import { describeBackend, sameBackend } from '@/lib/connectionDisplay'
import { devPreview } from '@/lib/devPreview'
import { localDeviceLabel } from '@/lib/deviceLabel'

const EASE = [0.22, 1, 0.36, 1] as const

const launchPreview = devPreview('launch')
const previewStatus: ConnectionStatus | null =
  launchPreview === null ? null : launchPreview === 'welcome' ? 'disconnected' : 'checking'

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

type SsoTarget = { url: string; label: string; oidcLoginUrl: string }

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

type LaunchScreenProps = { manual?: false; onClose?: never } | { manual: true; onClose: () => void }

export function LaunchScreen({ manual = false, onClose }: LaunchScreenProps = {}) {
  const { status: liveStatus, error, url: currentUrl, auth: currentAuth, oidcLoginUrl: currentOidcUrl } = useConnection()
  const status = previewStatus ?? liveStatus
  const [mode, setMode] = useState<'options' | 'remote' | 'sso'>('options')
  const [ssoTarget, setSsoTarget] = useState<SsoTarget | null>(null)
  const [awaitingBrowserSso, setAwaitingBrowserSso] = useState(false)
  const [busy, setBusy] = useState<string | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)
  const backends = useKnownBackends()
  const deviceLabel = localDeviceLabel()
  const current = describeBackend(currentUrl)
  const [url, setUrl] = useState('')
  const canStartLocal = clientRuntime.capabilities.localBackend
  const cloudOnly = !canStartLocal
  const visibleBackends = cloudOnly ? backends.filter((backend) => !describeBackend(backend.url).local) : backends
  const managedCloud = visibleBackends.find((backend) => backend.managed || backend.mode === 'cloud')

  const run = async (target: string, action: () => Promise<string | null>, closeOnSuccess = true) => {
    setBusy(target)
    setActionError(null)
    try {
      const err = await action()
      if (err) {
        setActionError(err)
      } else if (closeOnSuccess && currentConnection().status === 'connected') {
        onClose?.()
      }
    } catch (error) {
      setActionError(error instanceof Error ? error.message : 'Could not open the connection')
    } finally {
      setBusy(null)
    }
  }

  const onStartLocal = () => run('local', startLocal)

  const connectTo = (target: string) =>
    run(target, async () => {
      const err = await connectRemote(target)
      if (!err) return null
      const next = currentConnection()
      if (next.auth === 'oidc' && sameBackend(next.url, target)) {
        setSsoTarget({
          url: next.url,
          label: describeBackend(next.url).title,
          oidcLoginUrl: next.oidcLoginUrl || '/v1/auth/login',
        })
        setMode('sso')
        setAwaitingBrowserSso(false)
        return null
      }
      return err
    })

  const launchSso = (target: SsoTarget) =>
    run(
      `sso:${target.url}`,
      async () => {
        const err = await startCloudSignIn(target.url, target.oidcLoginUrl)
        if (err) return err
        if (clientRuntime.kind === 'electron') {
          setSsoTarget(target)
          setMode('sso')
          setAwaitingBrowserSso(true)
        }
        return null
      },
      false,
    )

  const isCurrent = (target: string) => manual && sameBackend(currentUrl, target)

  const checkingCopy =
    connectionPreference()?.mode === 'remote' || cloudOnly
      ? 'Connecting to Jaz Cloud…'
      : connectionPreference()?.mode === 'local' && canStartLocal
        ? `Starting jaz on ${deviceLabel}…`
        : 'Connecting to backend…'

  const reconnectHint = canStartLocal
    ? 'Start one here or connect to a cloud server.'
    : 'Sign in to connect to your Jaz Cloud workspace.'

  const ssoPromptNeeded = cloudOnly && currentAuth === 'oidc' && error === 'Sign in to connect to this server'
  const visibleError = ssoPromptNeeded ? null : error

  const heading = manual
    ? canStartLocal
      ? 'Switch workspace'
      : 'Connect to Jaz Cloud'
    : visibleError
      ? cloudOnly
        ? 'Sign in to Jaz Cloud'
        : 'Reconnect to jaz'
      : cloudOnly
        ? 'Welcome to Jaz Cloud'
        : 'Welcome to jaz'

  const discoveredSso: SsoTarget | null =
    !manual && currentAuth === 'oidc' && currentUrl && status === 'disconnected'
      ? {
          url: currentUrl,
          label: describeBackend(currentUrl).title,
          oidcLoginUrl: currentOidcUrl || '/v1/auth/login',
        }
      : null

  const activeSso =
    ssoTarget ??
    discoveredSso ??
    (cloudOnly && (clientRuntime.cloudUrl || managedCloud)
      ? {
          url: clientRuntime.cloudUrl || managedCloud!.url,
          label: managedCloud?.label || describeBackend(clientRuntime.cloudUrl).title,
          oidcLoginUrl: managedCloud?.oidcLoginUrl || currentOidcUrl || '/v1/auth/login',
        }
      : null)

  const signInBusyLabel = clientRuntime.kind === 'web' ? 'Signing in…' : 'Opening browser…'

  return (
    <div className="relative flex h-full flex-col bg-bg">
      <DitherTerrain className="absolute inset-x-0 bottom-0" delay={0.5} />
      <div className="titlebar-drag relative h-[52px] shrink-0">
        {manual && onClose ? (
          <button
            type="button"
            onClick={onClose}
            aria-label="Close"
            title="Close"
            className="absolute right-3 top-3 z-30 grid size-7 cursor-pointer place-items-center rounded-full text-ink-2 transition-colors duration-150 [-webkit-app-region:no-drag] hover:bg-surface-2 hover:text-ink"
          >
            <X size={16} />
          </button>
        ) : null}
      </div>
      <div className="relative flex flex-1 flex-col items-center justify-center px-6 pb-[52px]">
        <div className="mb-8">
          <DitherWordmark delay={0.35} />
        </div>
        <AnimatePresence mode="wait" initial={false}>
          {status === 'checking' ? (
            <motion.div
              key="checking"
              initial={{ opacity: 0 }}
              animate={{ opacity: 1, transition: { duration: 0.3, delay: 0.2 } }}
              exit={{ opacity: 0, transition: { duration: 0.15 } }}
              className="flex flex-col items-center gap-8"
            >
              <motion.p
                initial={{ opacity: 0 }}
                animate={{ opacity: 1, transition: { duration: 0.4, delay: 1.1 } }}
                className="text-[13px] text-ink-3"
              >
                {checkingCopy}
              </motion.p>
            </motion.div>
          ) : (
            <motion.div
              key="options"
              variants={stagger}
              initial="hidden"
              animate="show"
              className="flex w-full max-w-[340px] flex-col items-center"
            >
              <motion.h1
                variants={rise}
                className="text-balance text-center text-[22px] font-semibold tracking-tight text-ink"
              >
                {heading}
              </motion.h1>

              {manual ? (
                <motion.p variants={rise} className="mt-2 text-pretty text-center text-[13px] text-ink-2">
                  Currently on <span className="text-ink">{current.title}</span>.{' '}
                  {canStartLocal ? 'Run jaz locally or switch to a cloud workspace.' : 'Switch to another cloud server.'}
                </motion.p>
              ) : visibleError ? (
                <motion.p variants={rise} className="mt-2 text-pretty text-center text-[13px] text-ink-2">
                  <span>{visibleError}</span>
                  <span className="mt-1 block text-ink-3">{reconnectHint}</span>
                </motion.p>
              ) : cloudOnly && activeSso ? (
                <motion.p variants={rise} className="mt-2 text-pretty text-center text-[13px] text-ink-2">
                  Sign in to open <span className="text-ink">{activeSso.label}</span>.
                </motion.p>
              ) : null}

              <motion.div variants={rise} className="mt-6 w-full">
                <AnimatePresence mode="wait" initial={false}>
                  {mode === 'sso' && activeSso ? (
                    <motion.div
                      key="sso"
                      {...swap}
                      className="flex flex-col gap-2.5 rounded-[16px] bg-surface/90 p-3.5 shadow-[0_8px_30px_rgba(0,0,0,0.10)] backdrop-blur-[2px]"
                    >
                      <div className="flex items-center justify-between gap-2 px-0.5">
                        <span className="min-w-0 truncate text-[13px] font-medium text-ink">{activeSso.label}</span>
                        <span className="inline-flex shrink-0 items-center gap-1 rounded-full bg-primary-soft px-2 py-0.5 text-[11px] font-medium text-primary">
                          <Cloud size={11} />
                          Cloud
                        </span>
                      </div>
                      <ChoiceButton
                        primary
                        label="Sign in"
                        busyLabel={signInBusyLabel}
                        busy={busy === `sso:${activeSso.url}`}
                        disabled={busy !== null}
                        onClick={() => launchSso(activeSso)}
                      />
                      {awaitingBrowserSso ? (
                        <p className="px-0.5 text-center text-[11.5px] text-ink-3">
                          Complete sign-in in your browser — Jaz will connect automatically.
                        </p>
                      ) : null}
                      <div className="flex items-center justify-between px-0.5 pt-0.5">
                        <button
                          type="button"
                          onClick={() => {
                            setMode('options')
                            setAwaitingBrowserSso(false)
                            setActionError(null)
                          }}
                          className="cursor-pointer text-[12px] text-ink-3 transition-colors duration-150 hover:text-ink"
                        >
                          Back
                        </button>
                      </div>
                    </motion.div>
                  ) : mode === 'options' ? (
                    <motion.div key="opts" {...swap} className="flex flex-col gap-2">
                      {cloudOnly && activeSso && !isCurrent(activeSso.url) ? (
                        <ChoiceButton
                          primary
                          label="Sign in"
                          busyLabel={signInBusyLabel}
                          busy={busy === `sso:${activeSso.url}`}
                          disabled={busy !== null}
                          onClick={() => launchSso(activeSso)}
                        />
                      ) : null}
                      {canStartLocal ? (
                        <ChoiceButton
                          primary={!(manual && current.local)}
                          label={`Run on ${deviceLabel}`}
                          busyLabel="Starting backend…"
                          busy={busy === 'local'}
                          connected={manual && current.local}
                          kind="local"
                          disabled={busy !== null}
                          onClick={onStartLocal}
                        />
                      ) : null}
                      {visibleBackends
                        .filter((backend) => !(cloudOnly && activeSso && !isCurrent(activeSso.url) && sameBackend(backend.url, activeSso.url)))
                        .map((backend) => {
                        const isCloud = backend.mode === 'cloud' || backend.auth === 'oidc'
                        const connected = isCurrent(backend.url)
                        return (
                          <BackendChoice
                            key={backend.url}
                            label={backend.label}
                            busy={busy === backend.url || busy === `sso:${backend.url}`}
                            connected={connected}
                            managed={backend.managed}
                            isCloud={isCloud}
                            disabled={busy !== null}
                            onConnect={() => connectTo(backend.url)}
                            onForget={() => forgetBackend(backend.url)}
                          />
                        )
                      })}
                      {!cloudOnly ? (
                        <ChoiceButton
                          label="Connect to a server"
                          disabled={busy !== null}
                          onClick={() => setMode('remote')}
                        />
                      ) : activeSso || visibleBackends.length > 0 ? (
                        <button
                          type="button"
                          disabled={busy !== null}
                          onClick={() => setMode('remote')}
                          className="mt-1 cursor-pointer text-center text-[12px] text-ink-3 transition-colors duration-150 hover:text-ink disabled:opacity-50"
                        >
                          Use another server address
                        </button>
                      ) : (
                        <ChoiceButton
                          primary
                          kind="cloud"
                          label="Connect to Jaz Cloud"
                          disabled={busy !== null}
                          onClick={() => setMode('remote')}
                        />
                      )}
                    </motion.div>
                  ) : (
                    <motion.div
                      key="remote"
                      {...swap}
                      className="rounded-[16px] bg-surface/90 p-3 shadow-[0_8px_30px_rgba(0,0,0,0.10)] backdrop-blur-[2px]"
                    >
                      <RemoteServerForm
                        value={url}
                        cloudOnly={cloudOnly}
                        onChange={setUrl}
                        onSubmit={() => connectTo(url)}
                        onBack={() => {
                          setMode('options')
                          setActionError(null)
                        }}
                        busy={busy !== null}
                      />
                    </motion.div>
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

function ChoiceButton({
  label,
  busyLabel,
  busy = false,
  primary = false,
  connected = false,
  kind,
  disabled,
  onClick,
}: {
  label: string
  busyLabel?: string
  busy?: boolean
  primary?: boolean
  connected?: boolean
  kind?: 'local' | 'cloud'
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
      {busy ? (
        <LoaderCircle size={15} className="animate-spin" />
      ) : connected ? (
        <span className="size-1.5 shrink-0 rounded-full bg-ok" />
      ) : kind === 'local' ? (
        <Laptop size={14} className="shrink-0 opacity-80" />
      ) : kind === 'cloud' ? (
        <Cloud size={14} className="shrink-0 opacity-80" />
      ) : null}
      {busy && busyLabel ? busyLabel : label}
      {connected ? <span className="text-[12px] text-ink-3">Connected</span> : null}
    </motion.button>
  )
}

function BackendChoice({
  label,
  busy,
  connected,
  managed = false,
  isCloud = false,
  disabled,
  onConnect,
  onForget,
}: {
  label: string
  busy: boolean
  connected: boolean
  managed?: boolean
  isCloud?: boolean
  disabled: boolean
  onConnect: () => void
  onForget: () => void
}) {
  return (
    <div className="group/backend relative">
      <motion.button
        type="button"
        disabled={disabled}
        onClick={onConnect}
        whileTap={disabled ? undefined : { scale: 0.97 }}
        className="flex h-11 w-full cursor-pointer items-center justify-center gap-2 rounded-full bg-surface/90 px-4 text-[14px] font-medium text-ink shadow-[0_6px_20px_rgba(0,0,0,0.08)] backdrop-blur-[2px] transition-colors duration-150 enabled:hover:bg-surface disabled:cursor-default disabled:opacity-60"
      >
        {busy ? (
          <LoaderCircle size={15} className="shrink-0 animate-spin" />
        ) : connected ? (
          <span className="size-1.5 shrink-0 rounded-full bg-ok" />
        ) : isCloud ? (
          <Cloud size={14} className="shrink-0 text-primary" />
        ) : null}
        <span className="min-w-0 truncate">{label}</span>
        {isCloud && !connected ? (
          <span className="shrink-0 rounded-full bg-primary-soft px-1.5 py-0.5 text-[10px] font-medium text-primary">Cloud</span>
        ) : null}
        {connected ? <span className="shrink-0 text-[12px] text-ink-3">Connected</span> : null}
      </motion.button>
      {!connected && !managed ? (
        <button
          type="button"
          aria-label={`Forget ${label}`}
          title="Forget this server"
          disabled={disabled}
          onClick={onForget}
          className="absolute right-2 top-1/2 grid size-7 -translate-y-1/2 cursor-pointer place-items-center rounded-full text-ink-3 opacity-0 transition-colors duration-150 hover:bg-surface-2 hover:text-ink focus-visible:opacity-100 group-hover/backend:opacity-100 disabled:cursor-default"
        >
          <X size={14} />
        </button>
      ) : null}
    </div>
  )
}
