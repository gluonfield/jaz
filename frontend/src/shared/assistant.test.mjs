import { expect, test } from 'bun:test'
import { extractAssistantDeepLinkArg, parseAssistantAction, parseAssistantDeepLink } from './assistant'

test('parseAssistantDeepLink handles default, voice, dictate, and direct text URLs', () => {
  expect(parseAssistantDeepLink('jaz://assistant')).toEqual({ mode: 'default' })
  expect(parseAssistantDeepLink('jaz://assistant?mode=default')).toEqual({ mode: 'default' })
  expect(parseAssistantDeepLink('jaz://assistant?mode=voice')).toEqual({ mode: 'voice' })
  expect(parseAssistantDeepLink('jaz://assistant?voice=1')).toEqual({ mode: 'voice' })
  expect(parseAssistantDeepLink('jaz://assistant?mode=dictate')).toEqual({ mode: 'dictate' })
  expect(parseAssistantDeepLink('jaz://assistant?dictate=1')).toEqual({ mode: 'dictate' })
  expect(parseAssistantDeepLink('jaz://assistant?text=Check%20lead%20times')).toEqual({
    mode: 'send',
    text: 'Check lead times',
  })
  expect(parseAssistantDeepLink('jaz://assistant?mode=send&text=Add+a+task')).toEqual({
    mode: 'send',
    text: 'Add a task',
  })
})

test('parseAssistantDeepLink rejects unrelated schemes, hosts, unknown modes, and blank sends', () => {
  expect(parseAssistantDeepLink('https://assistant?mode=voice')).toBeNull()
  expect(parseAssistantDeepLink('jaz://app/1234567890abcdef')).toBeNull()
  expect(parseAssistantDeepLink('jaz://assistant?mode=unknown')).toBeNull()
  expect(parseAssistantDeepLink('jaz://assistant?mode=send&text=%20%20')).toBeNull()
  expect(parseAssistantDeepLink('not a url')).toBeNull()
})

test('extractAssistantDeepLinkArg finds the first valid assistant link in argv', () => {
  expect(extractAssistantDeepLinkArg(['/Applications/Jaz.app', '--flag', 'jaz://assistant?mode=dictate'])).toEqual({
    mode: 'dictate',
  })
  expect(extractAssistantDeepLinkArg(['/Applications/Jaz.app', 'jaz://app/abc'])).toBeNull()
})

test('parseAssistantAction validates IPC action payloads', () => {
  expect(parseAssistantAction({ mode: 'voice' })).toEqual({ mode: 'voice' })
  expect(parseAssistantAction({ mode: 'send', text: '  hello  ' })).toEqual({ mode: 'send', text: 'hello' })
  expect(parseAssistantAction({ mode: 'send', text: '   ' })).toBeNull()
  expect(parseAssistantAction({ mode: 'bogus' })).toBeNull()
})
