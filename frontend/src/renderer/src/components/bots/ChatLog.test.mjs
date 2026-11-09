import { expect, test } from 'bun:test'

test('bot chat renders attachments, mentions, and public group activity', async () => {
  const worker = new globalThis.Worker(new globalThis.URL('./ChatLog.worker.mjs', import.meta.url).href)
  try {
    const result = await new Promise((resolve, reject) => {
      worker.onmessage = (event) => resolve(event.data)
      worker.onerror = reject
    })
    for (const html of result.attachments) {
      expect(html).toContain('pasted-text-1.txt')
      expect(html).toContain('alt="screenshot.png"')
      expect(html).toContain('/v1/sessions/bot-1/attachments/image-1')
    }
    expect(result.attachments[1]).toContain('Read these')
    expect(result.mentions).toContain('href="/bots/b"')
    expect(result.mentions).toContain('href="/bots/c"')
    expect(result.mentions).toContain('title="Planner"')
    expect(result.mentions).toContain('href="/bots/f"')
    expect(result.mentions).toContain('href="/bots/g"')
    expect(result.mentions).toContain('href="/bots/h"')
    expect(result.mentions.match(/href="\/bots\//g)).toHaveLength(7)
    expect(result.mentions).toContain('[@Shared name] [@Missing] remain text.')
    expect(result.mentions).toContain('<code>[@Business Opportunist]</code>')
    expect(result.mentions).toContain('<code>[@Business Opportunist](bot:b)</code>')
    expect(result.groupProgress).toContain('Planner · Progress update')
    expect(result.groupProgress).toContain('The rollback check passed.')
    expect(result.groupProgress).toContain('Running a command')
    expect(result.groupProgress).toContain('terminal · › go')
    expect(result.groupProgress).not.toContain('Private notes')
  } finally {
    worker.terminate()
  }
})
