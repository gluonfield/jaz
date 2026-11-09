import { describe, expect, test } from 'bun:test'
import { deriveBotProgress } from './botProgress.ts'

describe('deriveBotProgress', () => {
  test('idle when not running', () => {
    expect(deriveBotProgress({ running: false, now: 1000 })).toBeUndefined()
  })
  test('does not infer reasoning from missing tool events', () => {
    const p = deriveBotProgress({ running: true, lastEventAt: new Date(900).toISOString(), now: 1000 })
    expect(p?.kind).toBe('processing')
    expect(p?.label).toBe('Waiting for activity details')
  })
  test('running command for execute tool', () => {
    const p = deriveBotProgress({
      running: true,
      lastEventAt: new Date(990).toISOString(),
      toolCalls: [{ id: '1', kind: 'execute', status: 'in_progress', updated_at: new Date(990).toISOString() }],
      now: 1000,
    })
    expect(p?.kind).toBe('running_command')
    expect(p?.detail).toBe('terminal')
  })
  test('reading for read tool', () => {
    const p = deriveBotProgress({
      running: true,
      lastEventAt: new Date(990).toISOString(),
      toolCalls: [{ id: '1', kind: 'read', status: 'in_progress', updated_at: new Date(990).toISOString() }],
      now: 1000,
    })
    expect(p?.kind).toBe('reading')
  })
  test('does not infer answer preparation from a completed tool', () => {
    const p = deriveBotProgress({
      running: true,
      lastEventAt: new Date(990).toISOString(),
      toolCalls: [{ id: '1', kind: 'read', status: 'completed', updated_at: new Date(990).toISOString() }],
      now: 1000,
    })
    expect(p?.kind).toBe('waiting')
    expect(p?.label).toBe('Waiting for the next event')
    expect(p?.detail).toBe('Last action: · file read')
  })
  test('marks missing fresh events stale without inventing a provider phase', () => {
    const p = deriveBotProgress({
      running: true,
      lastEventAt: new Date(1000).toISOString(),
      now: 200_000,
    })
    expect(p?.kind).toBe('processing')
    expect(p?.stale).toBe(true)
  })
  test('ageMs computed', () => {
    const now = 1000
    const p = deriveBotProgress({ running: true, lastEventAt: new Date(900).toISOString(), now })
    expect(p?.ageMs).toBe(100)
  })
  test('shows the active plan step alongside tool activity', () => {
    const p = deriveBotProgress({
      running: true,
      lastEventAt: new Date(990).toISOString(),
      toolCalls: [{ id: '1', tool_name: 'WebSearch', kind: 'search', status: 'in_progress', updated_at: new Date(990).toISOString() }],
      plan: [{ content: 'Find current API documentation', status: 'in_progress' }],
      now: 1000,
    })
    expect(p?.label).toBe('Searching the web')
    expect(p?.detail).toBe('web search · Find current API documentation')
  })
  test.each([
    'npm run build --token demo-token',
    'curl -u user:demo-password https://example.test',
    'curl --password "demo password with spaces" https://example.test',
    'curl -H "X-Api-Key: demo-key" https://example.test',
    'curl -H "Authorization: Bearer demo-token" https://example.test',
    'ssh host "TOKEN=demo-token command"',
    ['curl', '-u', 'user:demo-password', 'https://example.test'],
    ['bash', '-c', 'curl -H "X-Api-Key: demo-key" https://example.test'],
  ].map((command) => [command]))('shows only the known executable, never shell arguments: %j', (command) => {
    const p = deriveBotProgress({
      running: true,
      lastEventAt: new Date(990).toISOString(),
      toolCalls: [{ id: '1', kind: 'execute', status: 'in_progress', title: 'secret command', raw_input: { command }, updated_at: new Date(990).toISOString() }],
      now: 1000,
    })
    const json = JSON.stringify(p)
    expect(json).not.toContain('secret command')
    expect(json).not.toContain('demo')
    expect(json).not.toContain('example.test')
    const executable = Array.isArray(command) ? command[0] : command.split(' ')[0]
    expect(p?.detail).toBe(`terminal · › ${executable}`)
  })
  test.each([
    '/private/demo-secret/bin/tool --token demo-token',
    'DEMO_TOKEN=demo-token curl https://example.test',
    'unknown-demo-secret --password demo-password',
    'curl;echo demo-password',
    ['demo-secret', 'curl'],
    [{ name: 'curl', secret: 'demo-token' }],
  ].map((command) => [command]))('omits executable previews it cannot safely identify: %j', (command) => {
    const p = deriveBotProgress({
      running: true,
      now: 1000,
      toolCalls: [{ id: '1', kind: 'execute', status: 'in_progress', raw_input: { command } }],
    })
    expect(p?.detail).toBe('terminal')
  })
  test('shows a pending tool as active until it completes', () => {
    const p = deriveBotProgress({ running: true, now: 1000, toolCalls: [{ id: '1', kind: 'execute', status: 'pending' }] })
    expect(p?.kind).toBe('running_command')
    expect(p?.detail).toBe('terminal')
  })
  test.each([undefined, 'completed', 'failed', 'cancelled'])('does not call a tool active for status %s', (status) => {
    const p = deriveBotProgress({ running: true, now: 1000, toolCalls: [{ id: '1', kind: 'execute', status }] })
    expect(p?.kind).toBe('waiting')
    expect(p?.detail).toBe('Last action: · terminal')
  })
  test('describes a reported edit without claiming answer preparation', () => {
    const p = deriveBotProgress({ running: true, now: 1000, toolCalls: [{ id: '1', kind: 'edit', status: 'in_progress' }] })
    expect(p?.kind).toBe('editing')
    expect(p?.label).toBe('Editing files')
  })
  test('keeps an active tool visible when another tool completes more recently', () => {
    const p = deriveBotProgress({ running: true, now: 1000, toolCalls: [
      { id: 'still-running', kind: 'execute', status: 'in_progress', updated_at: new Date(900).toISOString() },
      { id: 'finished', kind: 'read', status: 'completed', updated_at: new Date(990).toISOString() },
    ] })
    expect(p?.kind).toBe('running_command')
    expect(p?.detail).toBe('terminal')
  })
})

describe('safe activity phase contract', () => {
  const at = (n) => new Date(n).toISOString()
  const activity = (phase, time, reason) => ({ phase, at: at(time), turn_started_at: at(1000), reason })
  test('first event wait resets old tools at the new turn boundary', () => {
    const p = deriveBotProgress({ running: true, now: 1200, activity: activity('starting', 1100), toolCalls: [
      { id: 'old', kind: 'execute', status: 'completed', updated_at: at(900) },
    ] })
    expect(p.kind).toBe('starting')
    expect(p.label).toBe('Waiting for the first agent event')
    expect(p.detail).toBeUndefined()
  })
  test.each([['reasoning', 'Reasoning'], ['responding', 'Responding']])('actual %s chunks have a phase without any tools', (phase, label) => {
    const p = deriveBotProgress({ running: true, now: 1500, activity: activity(phase, 1400) })
    expect(p.label).toBe(label)
    expect(p.ageMs).toBe(100)
    expect(p.detail).toBeUndefined()
  })
  test('the latest observed phase and tool timestamps determine current activity', () => {
    const toolCalls = [{ id: 'ssh', kind: 'execute', status: 'in_progress', program: 'ssh', updated_at: at(1300) }]
    expect(deriveBotProgress({ running: true, now: 1600, activity: activity('reasoning', 1200), toolCalls }).kind).toBe('running_command')
    const next = deriveBotProgress({ running: true, now: 1600, activity: activity('responding', 1500), toolCalls })
    expect(next.kind).toBe('responding')
    expect(next.detail).toBe('terminal · › ssh')
  })
  test('provider retry and failure override stale tools using safe reason enums', () => {
    const toolCalls = [{ id: 'ssh', kind: 'execute', status: 'in_progress', updated_at: at(1300) }]
    const retry = deriveBotProgress({ running: true, now: 1600, activity: activity('waiting', 1500, 'provider_retry'), toolCalls })
    expect(retry.kind).toBe('waiting')
    expect(retry.waitReason).toBe('The provider reported a retry')
    const error = deriveBotProgress({ running: true, now: 1600, activity: activity('error', 1500, 'provider_error'), toolCalls })
    expect(error.kind).toBe('error')
    expect(error.waitReason).toBe('The agent reported an error')
  })
  test('quiet time marks the last observed phase stale without inventing a new phase', () => {
    const p = deriveBotProgress({ running: true, now: 150000, activity: activity('reasoning', 1400) })
    expect(p.stale).toBe(true)
    expect(p.kind).toBe('reasoning')
    expect(p.waitReason).toBeUndefined()
  })
})
