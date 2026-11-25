import { createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { onlineManager, QueryClientProvider } from '@tanstack/react-query'
import { ReconnectingBanner } from '@/components/launch/LaunchScreen'
import { Composer } from '@/components/session/Composer'
import { installFileDropGuard } from '@/components/ui/FileDrop'
import { ToastProvider } from '@/components/ui/toast'
import { apiBaseUrl, apiEmbeddedGetUrl, apiEventSourceUrl, apiUrl, get, post, setApiBaseUrl } from '@/lib/api/client'
import { backendTransportUrl } from '@/lib/clientRuntime'
import { connectRemote, useConnection } from '@/lib/connection'
import { readHealth } from '@/lib/connectionProbe'
import { sessionMessagesQuery } from '@/lib/api/sessions'
import { queryClient } from '@/lib/query/queryClient'
import { submitNewSession } from '@/routes/-newSessionSubmission'
import { LOCAL_BACKEND_URL } from '@shared/localBackend'

function Status() {
  const { status } = useConnection()
  document.documentElement.dataset.status = status
  return createElement(ReconnectingBanner, { show: status === 'reconnecting' })
}

queryClient.mount()
createRoot(document.body).render(createElement(Status))
installFileDropGuard()
let submission: Promise<string> | undefined
Object.assign(window, {
  fixture: {
    showComposer: () => {
      const element = document.createElement('div')
      document.body.append(element)
      createRoot(element).render(createElement(QueryClientProvider, { client: queryClient },
        createElement(ToastProvider, null, createElement(Composer, {
          streaming: false,
          onSend: (text, options) => {
            let sessionId = ''
            submission = submitNewSession({}, text, options, (prompt) => { sessionId = prompt.sessionId })
              .then(() => sessionId, (error: Error) => error.message)
          },
        })),
      ))
    },
    get submission() { return submission },
    connect: () => connectRemote(LOCAL_BACKEND_URL + '#key=jaz_fixture'),
    health: () => readHealth(LOCAL_BACKEND_URL),
    chat: () => queryClient.fetchQuery(sessionMessagesQuery('stored-chat')),
    mutate: () => queryClient.getMutationCache().build(queryClient, { mutationFn: () => post('/fixture/mutate', { seen: true }) }).execute(undefined),
    remoteMutation: async (url: string) => {
      const original = apiBaseUrl()
      setApiBaseUrl(url)
      try {
        await queryClient.getMutationCache().build(queryClient, { mutationFn: () => post('/fixture/remote-mutate', { seen: true }) }).execute(undefined)
        return true
      } catch {
        return false
      } finally {
        setApiBaseUrl(original)
      }
    },
    get,
    post,
    apiUrl,
    apiBaseUrl,
    backendTransportUrl,
    embedded: () => apiEmbeddedGetUrl('/fixture/image'),
    stream: () => new Promise((resolve, reject) => {
      const source = new EventSource(apiEventSourceUrl('/fixture/events'))
      source.onmessage = (event) => {
        source.close()
        resolve(event.data)
      }
      source.onerror = () => {
        source.close()
        reject(new Error('Local event stream failed'))
      }
    }),
    offline: () => !onlineManager.isOnline(),
  },
})
