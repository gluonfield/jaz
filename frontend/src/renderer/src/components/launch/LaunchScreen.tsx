import { Globe, LoaderCircle, Play } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { type FormEvent, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { PixelField } from '@/components/ui/PixelField'
import { apiBaseUrl } from '@/lib/api/client'
import { connectRemote, rememberedRemoteUrl, startLocal, useConnection } from '@/lib/connection'
import { useTheme } from '@/lib/theme'

const EASE = [0.22, 1, 0.36, 1] as const

const stagger = {
  hidden: {},
  show: { transition: { staggerChildren: 0.08, delayChildren: 0.15 } },
}

const rise = {
  hidden: { opacity: 0, y: 14, filter: 'blur(6px)' },
  show: { opacity: 1, y: 0, filter: 'blur(0px)', transition: { duration: 0.5, ease: EASE } },
}

const swap = {
  initial: { opacity: 0, y: 8 },
  animate: { opacity: 1, y: 0 },
  exit: { opacity: 0, y: -8 },
  transition: { duration: 0.18, ease: 'easeOut' as const },
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
// renders the wordmark, so the chrome stays text-light.
export function LaunchScreen() {
  const { status, error } = useConnection()
  // PixelField samples the palette at mount; remount it when the theme flips.
  const { resolved } = useTheme()
  const [mode, setMode] = useState<'options' | 'remote'>('options')
  const [busy, setBusy] = useState<'local' | 'remote' | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)
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

  return (
    <div className="relative flex h-full flex-col bg-bg">
      {/* skip the GPU spin-up during the sub-second checking flash at boot */}
      {status === 'disconnected' && (
        <PixelField key={resolved} calm={mode === 'remote' || busy !== null} />
      )}
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
              <p className="text-[13px] text-ink-3">Connecting to backend…</p>
            </motion.div>
          ) : (
            <motion.div
              key="options"
              variants={stagger}
              initial="hidden"
              animate="show"
              className="flex w-full max-w-[380px] flex-col items-center gap-6"
            >
              <motion.p variants={rise} className="max-w-[300px] text-center text-[13px] text-ink-2">
                No backend is running. Start one on this machine or connect to a remote server.
              </motion.p>

              {error && (
                <motion.div
                  variants={rise}
                  className="w-full rounded-control bg-danger-soft px-3 py-2 text-center text-[12px] text-danger"
                >
                  {error}
                </motion.div>
              )}

              <motion.div variants={rise} className="w-full">
                <AnimatePresence mode="wait" initial={false}>
                  {mode === 'options' ? (
                    <motion.div key="opts" {...swap} className="flex flex-col items-center gap-2">
                      <Button
                        variant="primary"
                        size="lg"
                        disabled={busy !== null}
                        onClick={onStartLocal}
                      >
                        {busy === 'local' ? (
                          <LoaderCircle size={14} className="animate-spin" />
                        ) : (
                          <Play size={14} />
                        )}
                        {busy === 'local' ? 'Starting backend…' : 'Start locally'}
                      </Button>
                      <Button
                        variant="ghost"
                        size="lg"
                        disabled={busy !== null}
                        onClick={() => setMode('remote')}
                      >
                        <Globe size={14} />
                        Connect to remote
                      </Button>
                    </motion.div>
                  ) : (
                    <motion.form
                      key="remote"
                      {...swap}
                      onSubmit={onConnect}
                      className="mx-auto flex w-full max-w-[360px] flex-col gap-2.5"
                    >
                      <Input
                        autoFocus
                        value={url}
                        onChange={(e) => setUrl(e.target.value)}
                        placeholder="http://192.168.1.10:8080"
                        spellCheck={false}
                      />
                      <div className="flex items-center justify-end gap-2">
                        <Button
                          variant="ghost"
                          disabled={busy !== null}
                          onClick={() => setMode('options')}
                        >
                          Back
                        </Button>
                        <Button
                          variant="primary"
                          type="submit"
                          disabled={busy !== null || !url.trim()}
                        >
                          {busy === 'remote' && (
                            <LoaderCircle size={14} className="animate-spin" />
                          )}
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
                    className="text-center text-[12px] text-danger"
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
