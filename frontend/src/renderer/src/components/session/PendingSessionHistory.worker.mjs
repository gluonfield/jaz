import { mock } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'

mock.module('./Bubble', () => ({
  Bubble: ({ message }) => createElement('span', null, message.content),
}))

const { PendingSessionHistory } = await import('./PendingSessionHistory')
const html = renderToStaticMarkup(createElement(PendingSessionHistory, {
  sessionId: 'session-1',
  initialPrompt: {
    baselineMessageSeq: 0,
    message: {
      role: 'user',
      content: 'inspect this',
      blocks: [{ type: 'text', text: 'inspect this' }],
      created_at: '2026-08-17T10:00:00Z',
    },
  },
}))

globalThis.postMessage({
  showsPrompt: html.includes('inspect this'),
  showsSkeleton: html.includes('animate-pulse'),
})
