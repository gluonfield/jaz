import { beforeAll, expect, test } from 'bun:test'

let panels
beforeAll(async () => {
  const worker = new globalThis.Worker(new globalThis.URL('./sidebarNavigation.worker.mjs', import.meta.url).href)
  try {
    panels = await new Promise((resolve, reject) => {
      worker.onmessage = (event) => resolve(event.data)
      worker.onerror = reject
    })
  } finally {
    worker.terminate()
  }
})

for (const mobile of [true, false]) {
  test(`bots sidebar keeps ordinary task creation beside bot actions (${mobile ? 'mobile' : 'desktop'})`, () => {
    const { html, links } = mobile ? panels.mobileBots : panels.desktopBots
    expect(links).toContainEqual({ href: '/new', text: 'New task' })
    expect(html).toContain('aria-label="Loops"')
    expect(html).toContain('aria-label="New bot or group"')
    if (mobile) {
      expect(links).toContainEqual({ href: '/new', text: 'Chat' })
      expect(links).toContainEqual({ href: '/bots', text: 'Bots' })
      expect(links).toContainEqual({ href: '/boards', text: 'Boards' })
    }
  })
}

test('ordinary mobile sidebar retains task creation, search and section navigation', () => {
  const { html, links } = panels.mobileChats
  expect(links).toContainEqual({ href: '/new', text: 'New task' })
  expect(html).toContain('aria-label="Open search"')
  expect(links).toContainEqual({ href: '/new', text: 'Chat' })
  expect(links).toContainEqual({ href: '/bots', text: 'Bots' })
  expect(links).toContainEqual({ href: '/boards', text: 'Boards' })
})
