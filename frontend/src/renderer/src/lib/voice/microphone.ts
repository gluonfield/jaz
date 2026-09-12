export async function requestMicrophone(): Promise<MediaStream> {
  try {
    return await navigator.mediaDevices.getUserMedia({
      audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true },
    })
  } catch (error) {
    if (!(error instanceof DOMException) || error.name !== 'NotAllowedError') {
      throw error
    }
    let recovery = 'Allow microphone access in your browser’s site permissions and system settings, then try again.'
    if (window.jaz) {
      const appName = import.meta.env.DEV ? 'the terminal app running Jaz' : 'Jaz'
      recovery = /Mac/i.test(navigator.platform)
        ? `Enable ${appName} in System Settings → Privacy & Security → Microphone, then restart Jaz.`
        : 'Allow Jaz to use the microphone in your system privacy settings, then restart Jaz.'
    }
    throw new Error(`Microphone access is blocked. ${recovery}`, { cause: error })
  }
}
