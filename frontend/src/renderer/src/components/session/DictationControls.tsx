import { ArrowUp, LoaderCircle, Square, X } from 'lucide-react'
import { IconButton } from '@/components/ui/IconButton'
import type { useDictation } from '@/lib/hooks/useDictation'
import { DictationWaveform } from '@/components/session/DictationWaveform'

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
  return (
    <div className="flex h-10 items-center gap-2" data-escape-surface>
      <IconButton className="size-10" aria-label="Cancel dictation" title="Cancel dictation (Escape)" onClick={dictation.cancel}>
        <X size={18} />
      </IconButton>
      <div className="flex min-w-0 flex-1 items-center justify-center" role="status" aria-live="polite">
        <span className={recording ? 'sr-only' : 'truncate text-sm text-ink-3'}>{labels[phase]}</span>
        {recording ? <DictationWaveform level={dictation.level} /> : null}
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
