import { spawn } from 'node:child_process'
import { createInterface } from 'node:readline'
import { parseDictationEvent, type DictationEvent } from '../shared/dictation'

export function startDictationProcess(
  command: string,
  args: string[],
  emit: (event: DictationEvent) => void,
  onClose: () => void,
) {
  const child = spawn(command, args, { stdio: ['pipe', 'pipe', 'ignore'] })
  const lines = createInterface({ input: child.stdout })
  let finished = false
  let stopping = false
  let timeout: ReturnType<typeof setTimeout> | undefined

  const dispose = () => {
    if (finished) {
      return
    }
    finished = true
    clearTimeout(timeout)
    lines.close()
    child.kill('SIGKILL')
    onClose()
  }
  const fail = (message: string) => {
    if (finished) {
      return
    }
    emit({ type: 'error', message })
    dispose()
  }
  const deadline = (ms: number, message: string) => {
    clearTimeout(timeout)
    timeout = setTimeout(() => fail(message), ms)
  }
  deadline(120_000, 'Dictation could not start. Check microphone access and try again.')
  child.on('error', () => fail('Dictation could not start. Rebuild or update Jaz.'))
  child.stdin.on('error', () => fail('Dictation disconnected. Please try again.'))
  child.on('close', () => {
    if (!finished) {
      fail('Dictation ended before a transcript was ready. Please try again.')
    }
  })
  lines.on('line', (line) => {
    if (finished) {
      return
    }
    try {
      const event = parseDictationEvent(JSON.parse(line))
      if (event.type === 'status' && !stopping) {
        clearTimeout(timeout)
        if (event.phase === 'downloading') {
          deadline(600_000, 'The speech model download timed out. Check your connection and try again.')
        }
      }
      emit(event)
      if (event.type === 'complete' || event.type === 'error') {
        dispose()
      }
    } catch {
      fail('Dictation returned an invalid response. Please try again.')
    }
  })
  return {
    cancel: dispose,
    stop: () => {
      if (finished || stopping) {
        return
      }
      stopping = true
      child.stdin.write('stop\n')
      deadline(30_000, 'Transcription timed out. Please try again.')
    },
  }
}
