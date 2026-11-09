import { getVoiceSettings, connectVoice, sendVoiceTask, saveVoiceDiagnostics, saveVoiceTranscript } from '@/lib/api/liveVoice'
import { getSessionMessagesPage } from '@/lib/api/sessions'
import { openSessionEvents } from '@/lib/api/sse'
import { requestMicrophone } from '@/lib/voice/microphone'
import type { VoiceHost } from '@/lib/voice/host'

export function createDesktopVoiceHost(): VoiceHost {
  return {
    getSettings: getVoiceSettings,
    connect: connectVoice,
    readSession: (sessionId, signal) => getSessionMessagesPage(sessionId, { turns: 32 }, signal),
    subscribeSession: openSessionEvents,
    sendTask: sendVoiceTask,
    saveDiagnostics: saveVoiceDiagnostics,
    saveTranscript: saveVoiceTranscript,
    requestMicrophone,
    claude: window.jaz?.claudeVoice,
  }
}
