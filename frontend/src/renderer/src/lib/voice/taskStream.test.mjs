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
  (activity, error) => states.push({ activity, error }))
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
  test('45 seconds of unchanged work stays quiet and a result forwards before 30 more seconds of work', async () => {
    const originalSet = globalThis.setTimeout
    const originalClear = globalThis.clearTimeout
    const timers = new Map()
    let time = 0
    let timerID = 0
    globalThis.setTimeout = (callback, ms) => {
      timers.set(++timerID, { callback, at: time + ms })
      return timerID
    }
    globalThis.clearTimeout = (id) => timers.delete(id)
    const advance = (ms) => {
      const end = time + ms
      for (;;) {
        const next = [...timers.entries()].sort((a, b) => a[1].at - b[1].at)[0]
        if (!next || next[1].at > end) {
          break
        }
        time = next[1].at
        timers.delete(next[0])
        next[1].callback()
      }
      time = end
    }
    const f = fixture()
    f.setRead(async () => ({ ...structuredClone(disk), session: { ...disk.session, source_type: 'bot' }, events: [], latest_event_seq: 0 }))
    try {
      await f.stream.context()
      f.stream.start()
      f.stream.follow(f.task())
      await f.stream.refresh()
      const before = [...f.sent]
      advance(45000)
      expect(f.sent).toEqual(before)
      f.emit({ seq: 100, session_id: disk.session.id, type: 'room_message', at: disk.events.at(-1).at,
        room_message: { speaker: 'bot', bot_id: disk.session.id, text: 'Memory search and a clarification; no research thread yet.' } })
      expect(f.sent.filter((item) => item.speak).map((item) => item.text)).toEqual(['Memory search and a clarification; no research thread yet.'])
      expect(time).toBe(45000)
      expect(f.states.at(-1).activity).toBe('working')
      const forwarded = [...f.sent]
      advance(30000)
      expect(f.sent).toEqual(forwarded)
      expect(f.states.at(-1).activity).toBe('working')
      f.emit({ ...disk.events.at(-1), seq: 101 })
      expect(f.sent.filter((item) => item.speak)).toHaveLength(1)
      expect(f.states.at(-1).activity).toBe(null)
    } finally {
      f.stream.stop()
      globalThis.setTimeout = originalSet
      globalThis.clearTimeout = originalClear
    }
  })

  test('a bot rename updates voice identity during work without restarting or repeating it', async () => {
    const f = fixture()
    const snapshot = structuredClone(disk)
    snapshot.events = []
    snapshot.latest_event_seq = 0
    snapshot.session.source_type = 'bot'
    snapshot.session.title = 'Prime'
    f.setRead(async () => structuredClone(snapshot))
    await f.stream.context()
    f.stream.start()
    f.stream.follow(f.task())
    await f.stream.refresh()
    const before = f.sent.length
    snapshot.session.title = 'CAS soft'
    f.emit({ session_id: snapshot.session.id, type: 'session' })
    await f.stream.refresh()
    expect(f.sent.slice(before)).toEqual([{
      text: 'Current chat identity: the user renamed you to "CAS soft". This is now your own name.',
      speak: false,
      id: undefined,
    }])
    expect(f.closed()).toBe(false)
    expect(f.states.at(-1).activity).toBe('working')
    await f.stream.refresh()
    expect(f.sent.length).toBe(before + 1)
    snapshot.session.title = 'Prime again'
    await f.stream.refresh()
    expect(f.sent.at(-1).text).toContain('"Prime again"')
    f.stream.stop()
  })

  test('renaming an ordinary thread does not rename the voice assistant', async () => {
    const f = fixture()
    const snapshot = structuredClone(disk)
    snapshot.session.source_type = ''
    snapshot.session.title = 'Old topic'
    f.setRead(async () => structuredClone(snapshot))
    await f.stream.context()
    f.stream.start()
    snapshot.session.title = 'New topic'
    f.emit({ session_id: snapshot.session.id, type: 'session' })
    await f.stream.refresh()
    expect(f.sent.some(({ text }) => text.startsWith('Current chat identity:'))).toBe(false)
    f.stream.stop()
  })

  test('tool changes preserve activity without feeding repeated working messages to voice', async () => {
    const f = fixture()
    await f.stream.context()
    f.stream.start()
    f.stream.follow(f.task())
    await f.stream.refresh()
    const initial = [...f.sent]
    const event = disk.events[0]
    for (let index = 0; index < 6; index += 1) {
      f.emit({ ...event, seq: 100 + index, type: 'acp_tool', content: undefined,
        projection_key: `tool:${index}`, projection_op: 'replace',
        acp: { ...event.acp, tool_calls: [{ id: String(index), title: `Check ${index}`, status: 'completed' }] },
      })
    }
    await f.stream.refresh()
    expect(f.sent.filter((item) => !item.text.startsWith('Recorded agent tool calls:'))).toEqual(initial)
    expect(f.sent.at(-1).text).toContain('Check 5: completed')
    expect(f.sent.every((item) => !item.speak)).toBe(true)
    expect(f.states.at(-1)).toEqual({ activity: 'working', error: '' })
    f.stream.stop()
  })

  test('a sentence at end of message and an unpunctuated tail stream without waiting for more output', async () => {
    const f = fixture()
    await f.stream.context()
    f.stream.start()
    f.stream.follow(f.task())
    await f.stream.refresh()
    const event = disk.events.findLast((event) => event.type === 'acp_message')
    const sentence = 'The price range was an estimate, not a supplier quote.'
    f.emit({ ...event, content: sentence })
    await delay(250)
    expect(f.sent.filter((item) => item.speak).map((item) => item.text).join('')).toBe(sentence)
    f.emit({ ...event, seq: event.seq + 1, content: ' The verified price is' })
    await delay(250)
    expect(f.sent.filter((item) => item.speak).map((item) => item.text).join('')).toBe(`${sentence} The verified price is`)
    expect(f.states.at(-1).activity).toBe('working')
    f.stream.stop()
  })

  test('continuous deltas flush in batches and stopping discards the pending batch', async () => {
    const f = fixture()
    await f.stream.context()
    f.stream.start()
    f.stream.follow(f.task())
    await f.stream.refresh()
    const event = disk.events.findLast((event) => event.type === 'acp_message')
    const parts = ['The disk ', 'has forty-two ', 'gigabytes free']
    for (const [index, content] of parts.entries()) {
      f.emit({ ...event, seq: 100 + index, content })
      await delay(80)
    }
    expect(f.sent.filter((item) => item.speak).map((item) => item.text)).toEqual([parts.join('')])
    f.emit({ ...event, seq: 103, content: ' out of one terabyte.' })
    f.stream.stop()
    const count = f.sent.length
    await delay(250)
    expect(f.sent).toHaveLength(count)
  })

  test('streaming text publishes activity transitions without republishing each token', async () => {
    const f = fixture()
    await f.stream.context()
    f.stream.start()
    f.stream.follow(f.task())
    expect(f.states).toEqual([{ activity: 'working', error: '' }])
    await f.stream.refresh()
    f.states.length = 0
    const event = disk.events[0]
    for (let seq = event.seq; seq < event.seq + 30; seq += 1) {
      f.emit({ ...event, seq, type: 'acp_thought', content: 'Thinking', projection_key: 'thought' })
    }
    expect(f.states).toEqual([{ activity: 'thinking', error: '' }])
    f.connect(false)
    f.connect(false)
    expect(f.states).toHaveLength(2)
    expect(f.states.at(-1).error).toContain('Reconnecting')
    await f.stream.refresh()
    expect(f.states).toHaveLength(3)
    expect(f.states.at(-1)).toEqual({ activity: 'thinking', error: '' })
    f.stream.stop()
  })

  test('the current root task switches thinking to working while reasoning stays out of spoken context', async () => {
    const f = fixture()
    await f.stream.context()
    f.stream.start()
    f.stream.follow(f.task())
    await f.stream.refresh()
    const event = disk.events[0]
    const thought = { ...event, type: 'acp_thought', content: 'Private reasoning', projection_key: 'thought' }
    const tool = { ...event, seq: event.seq + 1, type: 'acp_tool', projection_key: 'tool', content: undefined }
    f.emit(thought)
    expect(f.states.at(-1).activity).toBe('thinking')
    f.emit({ ...tool, acp: { ...tool.acp, id: 'child', parent_id: disk.session.id } })
    expect(f.states.at(-1).activity).toBe('thinking')
    f.emit(tool)
    expect(f.states.at(-1).activity).toBe('working')
    f.emit(disk.events.findLast((event) => event.type === 'acp'))
    expect(f.states.at(-1).activity).toBe(null)
    expect(f.sent.some((item) => item.text.includes(thought.content))).toBe(false)
    f.stream.stop()
  })

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
    await delay(250)
    expect(f.sent.filter((item) => item.speak).map((item) => item.text).join('')).toContain('42 GB free')
    expect(f.states.at(-1).activity).toBe('working')
    expect(f.reads()).toBe(reads)
    f.emit(terminal)
    f.emit(final)
    expect(f.sent.filter((item) => item.speak).map((item) => item.text).join('')).toBe(final.content)
    expect(f.states.at(-1).activity).toBe(null)
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
    await delay(250)
    f.connect(false)
    expect(f.states.at(-1).error).toContain('Reconnecting')
    f.setRead(async () => structuredClone(disk))
    f.connect(true)
    await f.stream.refresh()
    expect(f.sent.filter((item) => item.speak).map((item) => item.text).join('')).toBe(final.content)
    expect(f.states.at(-1)).toEqual({ activity: null, error: '' })
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
    await delay(250)
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
    expect(f.states.at(-1).activity).toBe('working')
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
    await delay(250)
    expect(f.sent.filter((item) => item.speak)).toEqual([])
    expect(f.sent.at(-1).text).toContain('superseded')
    expect(f.states.at(-1).activity).toBe(null)
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
    expect(spoken).toBe('Disk check: 42 GB free. ')
    f.stream.stop()
  })

  test('a bot reply is forwarded immediately while narration stays private', async () => {
    const f = fixture()
    const bot = { ...structuredClone(disk), session: { ...disk.session, source_type: 'bot' }, events: [], latest_event_seq: 0 }
    f.setRead(async () => structuredClone(bot))
    await f.stream.context()
    f.stream.start()
    f.stream.follow(f.task())
    await f.stream.refresh()
    const narration = disk.events.findLast((event) => event.type === 'acp_message')
    f.emit({ ...narration, seq: 100, content: 'I’ll check your CRM now.' })
    await delay(250)
    expect(f.sent.filter((item) => item.speak)).toEqual([])
    f.emit({ seq: 101, session_id: disk.session.id, type: 'room_message', at: narration.at,
      room_message: { speaker: 'bot', bot_id: disk.session.id, name: 'Lead Generator', text: 'One deal: Park Place, at Lead.' } })
    expect(f.sent.filter((item) => item.speak).map((item) => item.text)).toEqual(['One deal: Park Place, at Lead.'])
    f.stream.stop()
  })
})
