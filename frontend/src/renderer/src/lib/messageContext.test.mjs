import { expect, test } from 'bun:test'
import { preparedSendMessage } from '@/lib/sendMessage'
import { normalizeQueuedMessageInput } from '@/lib/sessionQueue'

test('sending and queueing canvas selections keeps identical node ids on different pages distinct', () => {
  const contexts = [
    { id: 'pricing-cost', type: 'canvas', page: 'pricing plan', node: 'cost', label: 'Price per part' },
    { id: 'sales-cost', type: 'canvas', page: 'sales plan', node: 'cost', label: 'Sales cost' },
  ]
  const prepared = preparedSendMessage({ contexts })
  const queued = normalizeQueuedMessageInput(JSON.parse(JSON.stringify({ text: 'Compare these', contexts: prepared.contexts })))
  expect(queued.contexts).toEqual([
    { type: 'canvas', page: 'pricing plan', id: 'cost', text: 'Price per part' },
    { type: 'canvas', page: 'sales plan', id: 'cost', text: 'Sales cost' },
  ])
})

test('queued legacy canvas selections reopen on the migrated first page', () => {
  const queued = normalizeQueuedMessageInput({ text: '', contexts: [{ type: 'canvas', id: 'cost', text: 'Old cost' }] })
  expect(queued.contexts).toEqual([{ type: 'canvas', page: 'page-1', id: 'cost', text: 'Old cost' }])
})

test('an unselected active page accompanies a request without becoming an empty request', () => {
  const prepared = preparedSendMessage({ contexts: [{ id: 'page-location', type: 'canvas', page: 'layout', label: 'Design' }] })
  const contexts = JSON.parse(JSON.stringify(prepared.contexts))
  const queued = normalizeQueuedMessageInput({ text: 'Add here', contexts })
  expect(JSON.parse(JSON.stringify(queued.contexts))).toEqual([{ type: 'canvas', page: 'layout', text: 'Design' }])
  expect(normalizeQueuedMessageInput({ text: '', contexts })).toBeNull()
  expect(normalizeQueuedMessageInput({ text: 'Add here', contexts: [{ type: 'canvas', page: '', text: 'No page' }] }).contexts).toBeUndefined()
})
