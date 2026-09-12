import { useState, type CSSProperties } from 'react'
import { createRoot } from 'react-dom/client'
import { SidePanelControl, useSidePanelState } from '@/components/session/SidePanelState'
import { SidePanelResizeHandle } from '@/components/session/SidePanelResizeHandle'
import { PreviewPanel } from '@/components/session/PreviewPanel'
import { BrowserSessions, BrowserSessionsContext } from '@/lib/browserSessions'
import { SidebarVisibility } from '@/lib/sidebar'
import { setThemePref } from '@/lib/theme'

export async function exerciseBrowserLayout(): Promise<void> {
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;z-index:100;background:var(--color-bg)'
  document.body.append(element)
  const root = createRoot(element)
  const sessions = new BrowserSessions()
  sessions.update('layout', { target: { sourceUrl: location.origin + '/target', displayUrl: location.origin + '/target' } })
  function Chat() {
    const panel = useSidePanelState('layout')
    return <div ref={panel.containerRef} className="flex min-w-0 flex-1 flex-col">
      <header className="flex h-[52px] shrink-0 items-center justify-end px-4">
        <SidePanelControl open={panel.open} view={panel.view} sideChatAvailable={false} fileAvailable={false} onToggle={panel.toggle} onSelectView={panel.selectView} />
      </header>
      <div className="flex min-h-0 flex-1">
        <div className="min-w-0 flex-1 p-6 text-ink" data-layout-chat>
          <p className="text-[15px]">Open the browser to review the page alongside this conversation.</p>
        </div>
        {panel.open && panel.view === 'preview' ? <div className="relative shrink-0" data-layout-browser style={{ width: panel.width, '--side-panel-width': `${panel.width}px` } as CSSProperties}>
          <SidePanelResizeHandle width={panel.width} minWidth={panel.minWidth} maxWidth={panel.maxWidth} disabled={false} onResize={panel.resize} onResizeStart={() => panel.setResizing(true)} onResizeEnd={() => panel.setResizing(false)} />
          <PreviewPanel target={panel.previewTarget} onTargetChange={panel.setPreviewTarget} onClose={panel.toggle} />
        </div> : null}
      </div>
    </div>
  }
  function Fixture() {
    const [sidebar, setSidebar] = useState(true)
    return <BrowserSessionsContext.Provider value={sessions}><SidebarVisibility.Provider value={setSidebar}>
      <div className="flex h-full">
        {sidebar ? <nav className="w-[264px] shrink-0 bg-surface p-6 text-ink" aria-label="Main navigation">Jaz</nav> : null}
        <button className="absolute left-3 top-3 text-ink" aria-label="Toggle navigation" onClick={() => setSidebar((value) => !value)}>☰</button>
        <Chat />
      </div>
    </SidebarVisibility.Provider></BrowserSessionsContext.Provider>
  }
  const until = async (check: () => boolean | Promise<boolean>) => {
    const end = Date.now() + 5000
    while (!await check()) {
      if (Date.now() > end) {
        throw new Error('Browser layout check timed out')
      }
      await new Promise((resolve) => setTimeout(resolve, 20))
    }
  }
  const click = async (selector: string) => {
    const target = element.querySelector<HTMLElement>(selector)!
    if (!target) {
      throw new Error('Missing browser layout control: ' + selector)
    }
    const bounds = target.getBoundingClientRect()
    const x = Math.round(bounds.x + bounds.width / 2)
    const y = Math.round(bounds.y + bounds.height / 2)
    await window.smoke.pointer('mouseMove', x, y)
    await window.smoke.pointer('mouseDown', x, y)
    await window.smoke.pointer('mouseUp', x, y)
  }
  const browserWidth = () => element.querySelector('[data-layout-browser]')?.getBoundingClientRect().width ?? 0
  const chatWidth = () => element.querySelector('[data-layout-chat]')!.getBoundingClientRect().width
  try {
    await window.smoke.resize(1440, 900)
    await until(() => window.innerWidth === 1440)
    root.render(<Fixture />)
    await until(() => Boolean(element.querySelector('[title="Open Preview (⌘P)"]')))
    await click('[title="Open Preview (⌘P)"]')
    await until(() => !element.querySelector('nav') && browserWidth() === 864)
    await click('[aria-label="Toggle navigation"]')
    await until(() => Boolean(element.querySelector('nav')) && chatWidth() >= 360)
    await click('[aria-label="Toggle navigation"]')
    await until(() => !element.querySelector('nav') && browserWidth() === 864)
    const divider = element.querySelector<HTMLElement>('[role="separator"]')!
    const grip = divider.getBoundingClientRect()
    if (getComputedStyle(divider.lastElementChild!).backgroundColor === 'rgba(0, 0, 0, 0)') {
      throw new Error('The resize grip is invisible at rest')
    }
    const x = Math.round(grip.x + grip.width / 2)
    const y = Math.round(grip.y + grip.height / 2)
    await window.smoke.pointer('mouseMove', x, y)
    await window.smoke.pointer('mouseDown', x, y)
    await window.smoke.pointer('mouseMove', x - 100, y)
    await window.smoke.pointer('mouseUp', x - 100, y)
    await until(() => browserWidth() === 964)
    if (document.body.style.cursor || document.body.style.userSelect) {
      throw new Error('Resize left the page in dragging state')
    }
    await click('[role="separator"]')
    if (document.activeElement !== divider) {
      throw new Error('Clicking the divider did not focus it for keyboard resizing')
    }
    divider.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true }))
    await until(() => browserWidth() === 940)
    await click('[aria-label="Toggle navigation"]')
    await until(() => browserWidth() === 816 && chatWidth() >= 360)
    await new Promise((resolve) => setTimeout(resolve, 250))
    await window.smoke.capture('browser-navigation-open')
    await click('[aria-label="Toggle navigation"]')
    await until(() => browserWidth() === 940)
    setThemePref('dark')
    await new Promise((resolve) => setTimeout(resolve, 250))
    await window.smoke.capture('browser-layout-dark')
    setThemePref('light')
    await new Promise((resolve) => setTimeout(resolve, 250))
    await window.smoke.capture('browser-layout-light')
    await click('[title="Hide Preview panel (⌘P)"]')
    await until(() => browserWidth() === 0)
    await click('[aria-label="Toggle navigation"]')
    await until(() => Boolean(element.querySelector('nav')))
    await click('[title="Open Preview (⌘P)"]')
    await until(() => !element.querySelector('nav') && browserWidth() === 940)
    await window.smoke.resize(1050, 850)
    await until(() => browserWidth() === 690)
    if (chatWidth() < 360) {
      throw new Error('The browser squeezed the conversation below its minimum width')
    }
    await click('[aria-label="Toggle navigation"]')
    await until(() => browserWidth() === 426 && chatWidth() >= 360)
    element.querySelector('nav')!.style.width = '320px'
    await until(() => browserWidth() === 370 && chatWidth() >= 360)
  } catch (error) {
    await window.smoke.capture('browser-layout-failure')
    throw new Error(`${(error as Error).message}; viewport=${window.innerWidth}, browser=${browserWidth()}, sidebar=${Boolean(element.querySelector('nav'))}`, { cause: error })
  } finally {
    root.unmount()
    element.remove()
    await window.smoke.resize(1050, 850)
  }
}
