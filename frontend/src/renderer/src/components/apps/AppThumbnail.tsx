import { useQuery } from '@tanstack/react-query'
import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { RenderedMarkdown } from '@/components/session/MessageMarkdown'
import { appPreviewQuery, type JazApp } from '@/lib/api/apps'
import { buildAppSandboxDocument } from '@/lib/appBridge'
import { useArtifactThemeCSS } from '@/lib/hooks/useArtifactThemeCSS'

// The preview is laid out at the width of an inline artifact, then scaled down to its sheet.
const PAGE_WIDTH = 680

// The top of a card: a sheet of the app's content rising from a tinted field.
export function ThumbnailSheet({ children }: { children?: ReactNode }) {
  return (
    <div className="relative aspect-[16/10] overflow-hidden bg-ink/[0.045] dark:bg-ink/[0.06]">
      <div className="absolute inset-x-[9%] top-[12%] bottom-0 overflow-hidden rounded-t-[10px] bg-bg shadow-[0_1px_2px_rgb(0_0_0/0.05),0_8px_24px_-8px_rgb(0_0_0/0.14)] ring-1 ring-ink/[0.07] transition-transform duration-200 ease-[cubic-bezier(0.23,1,0.32,1)] group-hover/card:-translate-y-0.5 motion-reduce:transition-none">
        {children}
      </div>
    </div>
  )
}

// A live, inert miniature of a published app. Its content loads once the card
// scrolls into view; the app runs without its host bridge, so a preview can
// never store data, call tools or spend AI credits.
export function AppThumbnail({ app }: { app: JazApp }) {
  const sheet = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState(0)
  const [seen, setSeen] = useState(false)
  const theme = useArtifactThemeCSS()
  const preview = useQuery({ ...appPreviewQuery(app), enabled: seen })

  useEffect(() => {
    const element = sheet.current
    if (!element) return
    const resize = new ResizeObserver(([entry]) => setWidth(entry.contentRect.width))
    resize.observe(element)
    const visible = new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) {
        setSeen(true)
        visible.disconnect()
      }
    }, { rootMargin: '200px' })
    visible.observe(element)
    return () => {
      resize.disconnect()
      visible.disconnect()
    }
  }, [])

  const markdown = app.kind === 'document' && /\.(md|markdown)$/i.test(preview.data?.entryPath ?? '')
  const doc = useMemo(
    () => preview.data && !markdown ? buildAppSandboxDocument(app.title, preview.data.content, theme, [], [], { measureLayout: true }) : '',
    [app.title, markdown, preview.data, theme],
  )
  const scale = width / PAGE_WIDTH

  return (
    <ThumbnailSheet>
      <div ref={sheet} aria-hidden inert className="pointer-events-none size-full select-none">
        {preview.data && width ? (
          <div
            className="origin-top-left transition-opacity duration-300 starting:opacity-0"
            style={{ width: PAGE_WIDTH, height: `${100 / scale}%`, transform: `scale(${scale})` }}
          >
            {markdown ? (
              <div className="px-10 pt-8">
                <RenderedMarkdown text={preview.data.content.slice(0, 4000)} className="file-prose" />
              </div>
            ) : (
              <iframe
                title=""
                tabIndex={-1}
                sandbox="allow-scripts"
                referrerPolicy="no-referrer"
                srcDoc={doc}
                className="block size-full border-0 bg-transparent"
              />
            )}
          </div>
        ) : null}
      </div>
    </ThumbnailSheet>
  )
}
