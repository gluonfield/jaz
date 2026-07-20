import {
  ChevronRight,
  CircleEllipsis,
  ExternalLink,
  FilePenLine,
  FileText,
  Globe,
  Image,
  Search,
  SquareTerminal,
  type LucideIcon,
} from 'lucide-react'
import { memo, useState } from 'react'
import { Collapse } from '@/components/ui/Collapse'
import type { ACPToolCall } from '@/lib/api/types'
import {
  toolCallPresentation,
  toolDomain,
  type ToolCategory,
  type ToolPreview,
  type WebResult,
} from './toolPresentation'
import { EditDiffBlock, hasInlineDiff } from './EditDiffBlock'
import { hasToolRawDetails, ToolRawDetails } from './ToolRawDetails'
import { normalized } from './TranscriptUtils'

function openExternal(url: string): void {
  window.open(url, '_blank', 'noopener,noreferrer')
}

const Favicon = memo(function Favicon({ url }: { url: string }) {
  const [failed, setFailed] = useState(false)
  const domain = toolDomain(url)
  if (failed || !domain) return <Globe size={14} className="size-3.5 shrink-0 text-ink-3" aria-hidden />
  return (
    <img
      src={`https://www.google.com/s2/favicons?domain=${encodeURIComponent(domain)}&sz=64`}
      alt=""
      width={14}
      height={14}
      loading="lazy"
      onError={() => setFailed(true)}
      className="size-3.5 shrink-0 rounded-sm outline outline-1 outline-black/10 dark:outline-white/10"
    />
  )
})

const ResultRow = memo(function ResultRow({ url, title }: WebResult) {
  const domain = toolDomain(url)
  return (
    <button
      type="button"
      onClick={() => openExternal(url)}
      title={url}
      className="group flex min-h-9 w-full items-center gap-2 rounded-[8px] px-2 text-left transition-colors duration-150 hover:bg-surface motion-reduce:transition-none"
    >
      <Favicon url={url} />
      <span className="min-w-0 flex-1 truncate text-[12.5px] text-ink">{title || domain || url}</span>
      {domain && title.toLowerCase() !== domain.toLowerCase() ? (
        <span className="shrink-0 text-[11px] text-ink-3">{domain}</span>
      ) : null}
      <ExternalLink
        size={11}
        className="shrink-0 text-ink-3 opacity-0 transition-opacity group-hover:opacity-100"
        aria-hidden
      />
    </button>
  )
})

function HumanToolPreview({
  call,
  category,
  preview,
}: {
  call: ACPToolCall
  category: ToolCategory
  preview?: ToolPreview
}) {
  if (preview?.type === 'web_results') {
    return (
      <div
        role="group"
        aria-label={`${preview.total} search results; showing ${preview.items.length}`}
        className="mb-1 ml-7 flex flex-col rounded-control bg-surface/40 p-0.5 ring-1 ring-border/45"
      >
        {preview.items.map((result) => (
          <ResultRow key={result.url} {...result} />
        ))}
      </div>
    )
  }
  if (preview?.type === 'web_fetch') {
    return (
      <div className="mb-1 ml-7 rounded-control bg-surface/40 p-0.5 ring-1 ring-border/45">
        <ResultRow {...preview.item} />
      </div>
    )
  }
  if (category === 'edit' && hasInlineDiff(call)) {
    return (
      <div className="mb-1 ml-7">
        <EditDiffBlock call={call} />
      </div>
    )
  }
  return null
}

const categoryIcons: Record<ToolCategory, LucideIcon> = {
  command: SquareTerminal,
  edit: FilePenLine,
  image: Image,
  read: FileText,
  search: Search,
  tool: CircleEllipsis,
  web_fetch: Globe,
  web_search: Search,
}

export const ToolCallDetail = memo(function ToolCallDetail({ call }: { call: ACPToolCall }) {
  const [open, setOpen] = useState(false)
  const presentation = toolCallPresentation(call)
  const expandable = hasToolRawDetails(call.raw_input, presentation.output)
  const Icon = categoryIcons[presentation.category]
  const failed = normalized(call.status) === 'failed'
  return (
    <div className="relative min-w-0">
      <button
        type="button"
        disabled={!expandable}
        aria-expanded={expandable ? open : undefined}
        onClick={() => setOpen((value) => !value)}
        className="group flex min-h-8 w-full min-w-0 items-center gap-1.5 rounded-control text-left transition-colors duration-150 enabled:hover:text-ink disabled:cursor-default"
      >
        <span className="relative z-[1] flex size-5 shrink-0 items-center justify-center rounded-full bg-bg text-ink-3">
          <Icon size={12} aria-hidden />
        </span>
        <span className="min-w-0 flex-1 truncate text-[13px] text-ink-2">{presentation.label}</span>
        {presentation.meta ? (
          <span className={`shrink-0 text-[11.5px] tabular-nums ${failed ? 'text-danger' : 'text-ink-3'}`}>
            {presentation.meta}
          </span>
        ) : null}
        {expandable ? (
          <ChevronRight
            size={12}
            className={`mr-0.5 shrink-0 text-ink-3 transition-transform duration-150 motion-reduce:transition-none ${open ? 'rotate-90' : ''}`}
            aria-hidden
          />
        ) : null}
      </button>
      <Collapse open={open && expandable} className="w-full">
        <HumanToolPreview call={call} category={presentation.category} preview={presentation.preview} />
        <ToolRawDetails input={call.raw_input} output={presentation.output} />
      </Collapse>
    </div>
  )
})
