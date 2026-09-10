import { ArrowUp, LoaderCircle, Square, X } from 'lucide-react'
import { IconButton } from '@/components/ui/IconButton'
import type { useDictation } from '@/lib/hooks/useDictation'

const labels = {
  starting: 'Starting dictation…',
  downloading: 'Downloading speech model…',
  recording: 'Listening…',
  transcribing: 'Transcribing…',
}

export function DictationControls({
  dictation,
  canSend,
  queue,
}: {
  dictation: ReturnType<typeof useDictation>
  canSend: boolean
  queue: boolean
}) {
  const phase = dictation.phase
  if (!phase) {
    return null
  }
  const recording = phase === 'recording'
  const bars = [...Array<number>(72 - dictation.levels.length).fill(0), ...dictation.levels]
  const peak = Math.max(0.025, ...dictation.levels)
  return (
    <div className="flex h-10 items-center gap-2" data-escape-surface>
      <IconButton className="size-10" aria-label="Cancel dictation" title="Cancel dictation (Escape)" onClick={dictation.cancel}>
        <X size={18} />
      </IconButton>
      <div className="flex min-w-0 flex-1 items-center justify-center" role="status" aria-live="polite">
        <span className={recording ? 'sr-only' : 'truncate text-sm text-ink-3'}>{labels[phase]}</span>
        {recording ? (
          <svg className="h-8 w-full text-ink" viewBox="0 0 432 32" preserveAspectRatio="none" aria-hidden>
            {bars.map((level, index) => {
              const strength = Math.max(0, (level / peak - 0.12) / 0.88)
              const height = 3 + strength * 28
              return <rect key={index} x={index * 6 + 1} y={(32 - height) / 2} width={3} height={height} rx={1.5} fill="currentColor" opacity={(0.2 + strength * 0.65) * (0.5 + index / 142)} />
            })}
          </svg>
        ) : null}
      </div>
      <IconButton className="size-10 bg-surface-2" disabled={!recording} aria-label="Finish dictation" title="Finish dictation" onClick={() => void dictation.stop()}>
        {recording ? <Square size={13} fill="currentColor" strokeWidth={0} /> : <LoaderCircle size={18} className="animate-spin motion-reduce:animate-none" />}
      </IconButton>
      <IconButton className="size-10" variant="primary" disabled={!recording || !canSend} aria-label={queue ? 'Finish dictation and queue message' : 'Finish dictation and send message'} title={queue ? 'Finish and queue' : 'Finish and send'} onClick={() => void dictation.stop(true)}>
        <ArrowUp size={18} />
      </IconButton>
    </div>
  )
}
