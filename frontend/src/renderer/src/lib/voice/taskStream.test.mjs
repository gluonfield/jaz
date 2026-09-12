import { describe, expect, test } from 'bun:test'
import { setTimeout as delay } from 'node:timers/promises'
import { VoiceTaskStream } from './taskStream'
import disk from './testdata/disk-answer.json'

const deferred = () => {
  let resolve
  let reject
  const promise = new Promise((yes, no) => {
    resolve = yes
    reject = no
  })
  return { promise, resolve, reject }
}
const tick = () => delay(0)
const { structuredClone } = globalThis
const fixture = () => {
  const sent = []
  const states = []
  let emit
  let connection
  let reads = 0
  let closed = false
  let read = async () => ({ ...structuredClone(disk), events: [], latest_event_seq: 0 })
  const stream = new VoiceTaskStream({
    read: () => {
      reads += 1
      return read()
    },
    subscribe: (_after, onEvent, onConnection) => {
      emit = onEvent
      connection = onConnection
      return () => {
        closed = true
      }
    },
  }, disk.messages[0].blocks.find((block) => block.type === 'voice_context').id,
  (text, speak, id) => sent.push({ text, speak, id }),
  (working, error) => states.push({ working, error }))
  const task = () => {
    const context = disk.messages[0].blocks.find((block) => block.type === 'voice_context')
    return { id: context.request_id, callId: context.id }
  }
  return { stream, sent, states, task,
    emit: (event) => emit(structuredClone(event)),
    connect: (ready) => connection(ready),
    setRead: (next) => {
      read = next
    },
    reads: () => reads,
    closed: () => closed,
  }
}

describe('voice consumes the existing agent event stream', () => {
  test('the real disk answer reaches voice before idle, without polling or speaking it twice', async () => {
    const f = fixture()
    await f.stream.context()
    f.stream.start()
    f.stream.follow(f.task())
    await f.stream.refresh()
    const reads = f.reads()
    const final = disk.events.findLast((event) => event.type === 'acp_message')
    const terminal = disk.events.findLast((event) => event.type === 'acp')
    f.emit(final)
    expect(f.sent.filter((item) => item.speak).map((item) => item.text).join('')).toContain('42 GB free')
    expect(f.states.at(-1).working).toBe(true)
    expect(f.reads()).toBe(reads)
    f.emit(terminal)
    f.emit(final)
    expect(f.sent.filter((item) => item.speak).map((item) => item.text).join('')).toBe(final.content)
    expect(f.states.at(-1).working).toBe(false)
    expect(f.reads()).toBe(reads)
    f.stream.stop()
    expect(f.closed()).toBe(true)
  })

  test('a snapshot in flight cannot discard newer streamed text or inspect a newly admitted task', async () => {
    const f = fixture()
    await f.stream.context()
    f.stream.start()
    const old = deferred()
    const current = deferred()
    f.setRead(() => old.promise)
    const refreshing = f.stream.refresh()
    f.stream.follow(f.task())
    f.setRead(() => current.promise)
    const final = disk.events.findLast((event) => event.type === 'acp_message')
    const terminal = disk.events.findLast((event) => event.type === 'acp')
    f.emit(final)
    f.emit(terminal)
    old.resolve({ ...structuredClone(disk), session: { ...disk.session, status: 'idle' }, messages: [], events: [], latest_event_seq: 0 })
    await tick()
    expect(f.sent.some((item) => item.text.includes('superseded'))).toBe(false)
    current.resolve({ ...structuredClone(disk), events: [], latest_event_seq: 0 })
    await refreshing
    expect(f.sent.filter((item) => item.speak).map((item) => item.text).join('')).toBe(final.content)
    f.stream.stop()
  })

  test('reconnect replays only unseen output and stop closes the stream', async () => {
    const f = fixture()
    await f.stream.context()
    f.stream.start()
    f.stream.follow(f.task())
    await f.stream.refresh()
    const final = disk.events.findLast((event) => event.type === 'acp_message')
    f.emit(final)
    f.connect(false)
    expect(f.states.at(-1).error).toContain('Reconnecting')
    f.setRead(async () => structuredClone(disk))
    f.connect(true)
    await f.stream.refresh()
    expect(f.sent.filter((item) => item.speak).map((item) => item.text).join('')).toBe(final.content)
    expect(f.states.at(-1)).toEqual({ working: false, error: '' })
    f.stream.stop()
    const count = f.sent.length
    f.emit({ ...final, seq: 1000 })
    expect(f.sent).toHaveLength(count)
  })

  test('private reasoning and child output stay out; cancellation does not flush an unfinished answer', async () => {
    const f = fixture()
    await f.stream.context()
    f.stream.start()
    f.stream.follow(f.task())
    await f.stream.refresh()
    const final = disk.events.findLast((event) => event.type === 'acp_message')
    f.emit({ ...final, type: 'acp_thought', content: 'private' })
    f.emit({ ...final, acp: { ...final.acp, id: 'child', parent_id: disk.session.id }, content: 'child answer' })
    f.emit({ ...final, seq: final.seq + 1, content: 'The answer is still incomplete' })
    f.emit({ ...disk.events.at(-1), seq: final.seq + 2, acp: { ...final.acp, state: 'cancelled' } })
    const spoken = f.sent.filter((item) => item.speak).map((item) => item.text).join('')
    expect(spoken).toContain('cancelled')
    expect(spoken).not.toContain('incomplete')
    expect(spoken).not.toContain('private')
    expect(spoken).not.toContain('child answer')
    f.stream.stop()
  })

  test('real permission event shapes refresh approval state immediately', async () => {
    const f = fixture()
    await f.stream.context()
    f.stream.start()
    f.stream.follow(f.task())
    await f.stream.refresh()
    const permission = { id: 'approval', status: 'pending', title: 'Allow the command?' }
    f.setRead(async () => ({ ...structuredClone(disk), events: [], latest_event_seq: 0, acp_permissions: [permission] }))
    f.emit({ type: 'permission_request', session_id: disk.session.id, permission, at: new Date().toISOString() })
    await f.stream.refresh()
    expect(f.sent.at(-1).speak).toBe(true)
    expect(f.sent.at(-1).text).toContain('needs your approval')
    expect(f.states.at(-1).working).toBe(true)
    f.setRead(async () => ({ ...structuredClone(disk), events: [], latest_event_seq: 0, acp_permissions: [] }))
    f.emit({ type: 'permission_response', session_id: disk.session.id, permission: { ...permission, status: 'resolved' }, at: new Date().toISOString() })
    await f.stream.refresh()
    expect(f.sent.at(-1).speak).toBe(false)
    expect(f.sent.at(-1).text).toContain('working')
    f.stream.stop()
  })

  test('a newer instruction does not make the old unfinished sentence speakable', async () => {
    const f = fixture()
    await f.stream.context()
    f.stream.start()
    f.stream.follow(f.task())
    await f.stream.refresh()
    const reply = { ...disk.events.findLast((event) => event.type === 'acp_message'), content: 'You should delete' }
    f.emit(reply)
    f.setRead(async () => ({ ...structuredClone(disk), events: [reply], messages: [
      ...disk.messages,
      { seq: 2, role: 'user', content: 'Just list the files.', blocks: [], created_at: disk.events.at(-1).at },
    ] }))
    await f.stream.refresh()
    expect(f.sent.filter((item) => item.speak)).toEqual([])
    expect(f.sent.at(-1).text).toContain('superseded')
    expect(f.states.at(-1).working).toBe(false)
    f.stream.stop()
  })

  test('live agent activity prevents idle context replays while the stored status is stale', async () => {
    const f = fixture()
    f.setRead(async () => ({ ...structuredClone(disk), session: { ...disk.session, status: 'idle' }, messages: [], events: [], latest_event_seq: 0 }))
    await f.stream.context()
    f.stream.start()
    const reply = disk.events.findLast((event) => event.type === 'acp_message')
    f.emit(reply)
    expect(f.sent).toEqual([])
    f.emit(disk.events.at(-1))
    expect(f.sent).toHaveLength(1)
    expect(f.sent[0].speak).toBe(false)
    expect(f.sent[0].text).toContain('42 GB free')
    f.stream.stop()
  })

  test('refreshing during a text stream cannot append a coalesced prefix twice', async () => {
    const f = fixture()
    const final = disk.events.findLast((event) => event.type === 'acp_message')
    const first = { ...final, seq: 1, content: 'Disk check: ' }
    const initial = { ...structuredClone(disk), events: [first], latest_event_seq: 1 }
    f.setRead(async () => structuredClone(initial))
    await f.stream.context()
    f.stream.start()
    f.stream.follow(f.task())
    await f.stream.refresh()
    const pending = deferred()
    f.setRead(() => pending.promise)
    const refreshing = f.stream.refresh()
    f.emit({ ...final, seq: 2, content: '42 GB free. ' })
    pending.resolve(structuredClone(initial))
    await refreshing
    const spoken = f.sent.filter((item) => item.speak).map((item) => item.text).join('')
    expect(spoken).toBe('Disk check: 42 GB free.')
    f.stream.stop()
  })
})
