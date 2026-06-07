import { apiBaseUrl, ApiError } from './client'

export async function transcribeAudio(blob: Blob, signal?: AbortSignal): Promise<string> {
  const form = new FormData()
  form.append('file', blob, 'clip.webm')
  const res = await fetch(`${apiBaseUrl()}/v1/audio/transcribe`, {
    method: 'POST',
    body: form,
    signal,
  })
  if (!res.ok) {
    let message = `${res.status} ${res.statusText}`
    try {
      const body = (await res.json()) as { error?: string }
      if (body.error) message = body.error
    } catch {
      // keep status text
    }
    throw new ApiError(res.status, message)
  }
  const data = (await res.json()) as { text: string }
  return data.text
}

export async function speakStream(text: string, signal?: AbortSignal): Promise<Response> {
  const res = await fetch(`${apiBaseUrl()}/v1/audio/speak`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ text }),
    signal,
  })
  if (!res.ok || !res.body) {
    let message = `${res.status} ${res.statusText}`
    try {
      const body = (await res.json()) as { error?: string }
      if (body.error) message = body.error
    } catch {
      // keep status text
    }
    throw new ApiError(res.status, message)
  }
  return res
}
