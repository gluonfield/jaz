import { useQuery } from '@tanstack/react-query'
import { AppWindow, FileText } from 'lucide-react'
import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { RenderedMarkdown } from '@/components/session/MessageMarkdown'
import { Skeleton } from '@/components/ui/Skeleton'
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
                <RenderedMarkdown text={preview.data.content.slice(0, 4000)} className="file-prose" artifactPreview />
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
        ) : preview.isError ? (
          <AppThumbnailFallback app={app} />
        ) : seen ? (
          <div className="flex size-full flex-col gap-2.5 px-5 pt-5">
            <Skeleton className="h-3 w-2/5" />
            <Skeleton className="h-2.5 w-4/5" />
            <Skeleton className="h-2.5 w-3/5" />
          </div>
        ) : null}
      </div>
    </ThumbnailSheet>
  )
}

function AppThumbnailFallback({ app }: { app: JazApp }) {
  const iconValue = app.icon?.value?.trim()
  const FallbackIcon = app.kind === 'document' ? FileText : AppWindow
  const hue = [...app.id].reduce((sum, char) => sum + char.charCodeAt(0), 0) % 360
  const badgeStyle = iconValue
    ? {
        backgroundColor: app.icon?.background || `hsl(${hue} 44% 44% / 0.16)`,
        color: app.icon?.color || undefined,
      }
    : undefined

  return (
    <div className="grid size-full place-items-center pb-[6%]">
      <span
        className="grid size-10 place-items-center rounded-[10px] bg-surface-2 text-ink-2 ring-1 ring-border/70"
        style={badgeStyle}
      >
        {iconValue ? (
          <span className={app.icon?.kind === 'emoji' ? 'text-[20px] leading-none' : 'text-[13px] font-semibold tracking-tight'}>
            {iconValue}
          </span>
        ) : (
          <FallbackIcon size={18} />
        )}
      </span>
    </div>
  )
}
