import type { ComputerAPI } from '@shared/computerControl'
import { ComputerRepl } from '@/lib/computerRepl'

export function connectComputer(url: string, host: ComputerAPI, sessionId: string, protocols: string[] = []): () => void {
  const repl = new ComputerRepl(host, sessionId)
  let stopped = false
  let socket: WebSocket
  let retry: ReturnType<typeof setTimeout>
  let active: number | undefined
  const connect = () => {
    socket = new WebSocket(url, protocols)
    const current = socket
    current.onmessage = async (event) => {
      if (stopped) {
        return
      }
      let request: { id: number; method: string; params?: { id?: number; code?: string } }
      try {
        request = JSON.parse(event.data)
      } catch {
        current.close()
        return
      }
      if (request.method === 'Jaz.cancel') {
        if (request.params?.id === active) {
          repl.cancel()
        }
        return
      }
      try {
        let result
        if (request.method === 'Jaz.status') {
          const status = await host.status()
          result = { status: status.available ? 'connected' : 'unavailable', text: JSON.stringify({ ...status, connected: true }), data: status }
        } else if (request.method === 'Jaz.run') {
          if (active !== undefined) {
            throw new Error('Another computer script is running')
          }
          active = request.id
          try {
            result = await repl.run(request.params?.code ?? '')
          } finally {
            active = undefined
          }
        } else {
          throw new Error('Unsupported computer command')
        }
        if (current.readyState === WebSocket.OPEN) {
          current.send(JSON.stringify({ id: request.id, result }))
        }
      } catch (error) {
        if (current.readyState === WebSocket.OPEN) {
          current.send(JSON.stringify({ id: request.id, error: { code: -32000, message: (error instanceof Error ? error.message : String(error)).slice(0, 12000) } }))
        }
      }
    }
    current.onclose = () => {
      repl.cancel()
      if (!stopped) {
        retry = setTimeout(connect, 2000)
      }
    }
  }
  connect()
  return () => {
    stopped = true
    clearTimeout(retry)
    socket.close()
    repl.cancel()
  }
}
