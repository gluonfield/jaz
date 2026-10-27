import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { motion } from 'motion/react'
import { useLayoutEffect, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { SidePanelStateProvider, useSidePanelState } from '@/components/session/SidePanelState'
import { SidePanelControl } from '@/components/session/SidePanelControl'
import { SidePanelDrawer } from '@/components/session/SidePanelDrawer'
import { BrowserPanelSlot, BrowserWorkspace } from '@/components/browser/BrowserWorkspace'
import { useBrowserSessions } from '@/lib/browserSessions'
import { drawerSlide } from '@/lib/dom/drawer'
import { keys } from '@/lib/query/keys'
import { SidebarVisibility } from '@/lib/sidebar'
import { setThemePref } from '@/lib/theme'

export async function exerciseBrowserLayout(): Promise<void> {
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;background:var(--color-bg)'
  document.body.append(element)
  const root = createRoot(element)
  const queryClient = new QueryClient()
  queryClient.setQueryData(keys.browserSettings, { enabled: false, mode: 'desktop' })
  let setNavigationWidth: (width: number) => void
  function Chat() {
    const sessions = useBrowserSessions()
    const panel = useSidePanelState('layout')
    useLayoutEffect(() => {
      sessions.update('layout', { target: { sourceUrl: location.origin + '/target', displayUrl: location.origin + '/target' } })
    }, [sessions])
    return <div ref={panel.containerRef} className="flex min-w-0 flex-1 flex-col">
      <header className="flex h-[52px] shrink-0 items-center justify-end px-4">
        <SidePanelControl open={panel.open} mode={panel.mode} onToggle={panel.toggleMode} />
      </header>
      <div className="flex min-h-0 flex-1">
        <div className="min-w-0 flex-1 p-6 text-ink" data-layout-chat>
          <p className="text-[15px]">Open the browser to review the page alongside this conversation.</p>
        </div>
        <SidePanelDrawer panel={panel} isMobile={false}>
          {panel.mode === 'tabs'
            ? <BrowserPanelSlot sessionId="layout" visible={panel.open} onClose={panel.close} embedded />
            : <div className="h-full bg-bg p-6" style={{ width: panel.width }}>Overview</div>}
        </SidePanelDrawer>
      </div>
    </div>
  }
  function Fixture() {
    const [sidebar, setSidebar] = useState(true)
    const [sidebarWidth, setSidebarWidth] = useState(264)
    useLayoutEffect(() => {
      setNavigationWidth = setSidebarWidth
    }, [])
    return <QueryClientProvider client={queryClient}><BrowserWorkspace><SidePanelStateProvider><SidebarVisibility.Provider value={setSidebar}>
      <div className="app-chrome flex h-full">
        <motion.div className="shrink-0 overflow-hidden" initial={false} animate={drawerSlide({ isMobile: false, open: sidebar, side: 'left', width: sidebarWidth })} transition={{ type: 'spring', stiffness: 400, damping: 36 }}>
          <nav style={{ width: sidebarWidth }} className="h-full bg-surface p-6 text-ink" aria-hidden={!sidebar} aria-label="Main navigation">Jaz</nav>
        </motion.div>
        <button className="absolute left-3 top-3 text-ink" aria-label="Toggle navigation" onClick={() => setSidebar((value) => !value)}>☰</button>
        <Chat />
      </div>
    </SidebarVisibility.Provider></SidePanelStateProvider></BrowserWorkspace></QueryClientProvider>
  }
  const until = async (check: () => boolean | Promise<boolean>) => {
    const end = Date.now() + 5000
    while (!await check()) {
      if (Date.now() > end) {
        throw new Error('Browser layout check timed out: ' + check.toString())
      }
      await new Promise((resolve) => setTimeout(resolve, 20))
    }
  }
  const click = async (selector: string) => {
    const target = element.querySelector<HTMLElement>(selector)!
    if (!target) {
      throw new Error('Missing browser layout control: ' + selector)
    }
    const initial = target.getBoundingClientRect()
    await window.smoke.pointer('mouseMove', Math.round(initial.x + initial.width / 2), Math.round(initial.y + initial.height / 2))
    await new Promise((resolve) => setTimeout(resolve, 250))
    const bounds = target.getBoundingClientRect()
    const x = Math.round(bounds.x + bounds.width / 2)
    const y = Math.round(bounds.y + bounds.height / 2)
    await window.smoke.pointer('mouseMove', x, y)
    await window.smoke.pointer('mouseDown', x, y)
    await window.smoke.pointer('mouseUp', x, y)
  }
  const drawerElement = () => element.querySelector('[data-layout-chat]')!.nextElementSibling as HTMLElement
  const browserWidth = () => drawerElement().getBoundingClientRect().width
  const navigation = () => element.querySelector<HTMLElement>('nav:not([aria-hidden="true"])')
  const chatWidth = () => element.querySelector('[data-layout-chat]')!.getBoundingClientRect().width
  const animatePanel = async (selector: string, destination: number) => {
    const frames: { width: number; left: number; time: number }[] = []
    let frame = 0
    const record = () => {
      const bounds = drawerElement().getBoundingClientRect()
      frames.push({ width: bounds.width, left: bounds.left, time: performance.now() })
      frame = requestAnimationFrame(record)
    }
    record()
    try {
      await click(selector)
      await until(() => Math.abs(browserWidth() - destination) < 0.1)
      await new Promise(requestAnimationFrame)
    } finally {
      cancelAnimationFrame(frame)
    }
    const start = frames[0].width
    const distance = Math.abs(destination - start)
    const intermediate = frames.filter(({ width }) => Math.abs(width - start) > 2 && Math.abs(width - destination) > 2)
    const jumps = frames.slice(1).map((sample, index) => Math.abs(sample.left - frames[index].left))
    if (intermediate.length < 3 || Math.max(...jumps) > distance * 0.65) {
      console.warn(JSON.stringify({ panelFrames: frames }))
      throw new Error(`Panel snapped: ${start}px to ${destination}px, ${intermediate.length} intermediate frames, ${Math.max(...jumps)}px largest edge jump`)
    }
    console.warn(JSON.stringify({ panelAnimation: { start, destination, frames: frames.length, intermediate: intermediate.length, maxEdgeJump: Math.max(...jumps) } }))
  }
  try {
    await window.smoke.resize(1440, 900)
    await until(() => window.innerWidth === 1440)
    root.render(<Fixture />)
    await until(() => Boolean(element.querySelector('[title="Open Side Panel (⌘⇧S)"]')))
    await animatePanel('[title="Open Side Panel (⌘⇧S)"]', 864)
    await window.smoke.capture('browser-preview-opening')
    await until(() => !navigation() && browserWidth() === 864)
    await animatePanel('[title="Hide Side Panel (⌘⇧S)"]', 0)
    await animatePanel('[title="Open Side Panel (⌘⇧S)"]', 864)
    await window.smoke.capture('browser-preview-reopening')
    await animatePanel('[title="Open Overview (⌘O)"]', 300)
    await animatePanel('[title="Open Side Panel (⌘⇧S)"]', 864)
    await click('[aria-label="Toggle navigation"]')
    await until(() => Boolean(navigation()) && Math.abs(browserWidth() - 705.6) < 1 && chatWidth() >= 360)
    await click('[aria-label="Toggle navigation"]')
    await until(() => !navigation() && browserWidth() === 864)
    const divider = element.querySelector<HTMLElement>('[role="separator"]')!
    const grip = divider.getBoundingClientRect()
    if (getComputedStyle(divider.lastElementChild!).backgroundColor !== 'rgba(0, 0, 0, 0)') {
      throw new Error('The resize border should be hidden at rest')
    }
    const x = Math.round(grip.x + grip.width / 2)
    const y = Math.round(grip.y + grip.height / 2)
    if (!divider.contains(document.elementFromPoint(x, y))) {
      throw new Error('The browser covers the resize handle')
    }
    await window.smoke.pointer('mouseMove', x, y)
    await until(() => getComputedStyle(divider.lastElementChild!).backgroundColor !== 'rgba(0, 0, 0, 0)')
    await window.smoke.pointer('mouseDown', x, y)
    await window.smoke.pointer('mouseMove', x + 64, y)
    await until(() => browserWidth() === 800)
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
    await window.smoke.key('Right')
    await until(() => browserWidth() === 940)
    await window.smoke.key('Tab')
    await until(() => document.activeElement !== divider)
    await window.smoke.key('Tab', ['shift'])
    await window.smoke.capture('browser-divider-keyboard')
    await until(() => document.activeElement === divider && divider.matches(':focus-visible'))
    const focusedGrip = divider.getBoundingClientRect()
    const focusedX = Math.round(focusedGrip.x + focusedGrip.width / 2)
    await window.smoke.pointer('mouseMove', focusedX, y)
    await window.smoke.pointer('mouseDown', focusedX, y)
    await window.smoke.pointer('mouseMove', focusedX - 24, y)
    await until(() => browserWidth() === 964)
    await window.smoke.capture('browser-divider-drag')
    if (getComputedStyle(divider).outlineStyle !== 'none') {
      throw new Error(`The resize handle draws a second line: ${getComputedStyle(divider).outline}`)
    }
    await window.smoke.pointer('mouseUp', focusedX - 24, y)
    await window.smoke.key('Right')
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
    await click('[title="Hide Side Panel (⌘⇧S)"]')
    await until(() => browserWidth() === 0)
    await click('[aria-label="Toggle navigation"]')
    await until(() => Boolean(navigation()))
    await click('[title="Open Side Panel (⌘⇧S)"]')
    await until(() => !navigation() && browserWidth() === 940 && element.querySelector('nav')!.parentElement!.getBoundingClientRect().width === 0)
    await window.smoke.resize(1050, 850)
    await until(() => browserWidth() === 690)
    if (chatWidth() < 360) {
      throw new Error('The browser squeezed the conversation below its minimum width')
    }
    const sidebarColumn = element.querySelector('nav')!.parentElement!
    const drawer = element.querySelector('[role="separator"]')!.parentElement!
    const panelCard = drawer.lastElementChild!.firstElementChild!
    const browserHost = element.querySelector<HTMLElement>('[data-browser-session="layout"]')!
    const frames: { sidebar: number; drawer: number; expected: number; browserRight: number; browserLeft: number; cardRight: number; cardLeft: number }[] = []
    const record = () => {
      const column = drawer.getBoundingClientRect()
      const card = panelCard.getBoundingClientRect()
      const browser = browserHost.getBoundingClientRect()
      frames.push({
        sidebar: sidebarColumn.getBoundingClientRect().width,
        drawer: column.width,
        expected: Math.round(drawer.parentElement!.getBoundingClientRect().width) - 360,
        browserRight: browser.right,
        browserLeft: browser.left,
        cardRight: card.right,
        cardLeft: card.left,
      })
    }
    const observer = new ResizeObserver(record)
    observer.observe(sidebarColumn)
    observer.observe(drawer)
    try {
      await click('[aria-label="Toggle navigation"]')
      await until(() => browserWidth() === 426 && sidebarColumn.getBoundingClientRect().width === 264)
      const toggle = element.querySelector<HTMLButtonElement>('[aria-label="Toggle navigation"]')!
      toggle.click()
      await new Promise((resolve) => setTimeout(resolve, 80))
      toggle.click()
      await until(() => browserWidth() === 426 && sidebarColumn.getBoundingClientRect().width === 264)
      toggle.click()
      await until(() => browserWidth() === 690 && sidebarColumn.getBoundingClientRect().width === 0)
      toggle.click()
      await until(() => browserWidth() === 426 && sidebarColumn.getBoundingClientRect().width === 264)
    } finally {
      observer.disconnect()
    }
    const movingFrames = frames.filter((frame) => frame.sidebar > 1 && frame.sidebar < 263)
    const lag = Math.max(...frames.map((frame) => Math.abs(frame.drawer - frame.expected)))
    const edgeGap = Math.max(...frames.flatMap((frame) => [Math.abs(frame.browserRight - frame.cardRight), Math.abs(frame.browserLeft - frame.cardLeft)]))
    if (movingFrames.length < 8 || lag > 1 || edgeGap > 1) {
      throw new Error(`Browser resize was not synchronized: ${movingFrames.length} moving frames, ${lag}px width lag, ${edgeGap}px edge gap`)
    }
    console.warn(JSON.stringify({ layoutSynchronization: { movingFrames: movingFrames.length, maxWidthLag: lag, maxEdgeGap: edgeGap } }))
    setNavigationWidth(320)
    await until(() => browserWidth() === 370 && chatWidth() >= 360)
  } catch (error) {
    await window.smoke.capture('browser-layout-failure')
    throw new Error(`${(error as Error).message}; viewport=${window.innerWidth}, browser=${browserWidth()}, sidebar=${Boolean(navigation())}`, { cause: error })
  } finally {
    root.unmount()
    queryClient.clear()
    element.remove()
    await window.smoke.resize(1050, 850)
  }
}
