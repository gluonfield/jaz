import { apiWebSocket } from '@/lib/api/client'
import type { BrowserCommand } from '@shared/browserControl'

export function connectBrowser(path: string, execute: (request: BrowserCommand) => unknown, cancel: () => void = () => {}): () => void {
  let stopped = false
  let socket: WebSocket
  let retry: ReturnType<typeof setTimeout>
  const connect = () => {
    socket = apiWebSocket(path)
    const current = socket
    current.onmessage = async (event) => {
      if (stopped) return
      let request: BrowserCommand & { id: number }
      try {
        request = JSON.parse(event.data)
      } catch {
        current.close()
        return
      }
      try {
        const result = await execute(request)
        if (current.readyState === WebSocket.OPEN) {
          current.send(JSON.stringify({ id: request.id, result }))
        }
      } catch (error) {
        if (current.readyState === WebSocket.OPEN) {
          current.send(JSON.stringify({ id: request.id, error: { code: -32000, message: error instanceof Error ? error.message : String(error) } }))
        }
      }
    }
    current.onclose = () => {
      if (stopped) return
      cancel()
      retry = setTimeout(connect, 2000)
    }
  }
  connect()
  return () => {
    stopped = true
    clearTimeout(retry)
    socket.close()
    cancel()
  }
}
