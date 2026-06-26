import { Globe } from 'lucide-react'

function previewHost(url: string): string {
  try {
    return new URL(url).host || url
  } catch {
    return url
  }
}

export function PreviewSuggestion({ url, onOpen }: { url: string; onOpen: (url: string) => void }) {
  return (
    <button
      type="button"
      onClick={() => onOpen(url)}
      className="group flex w-full cursor-pointer items-center gap-3 rounded-card bg-surface px-3 py-2.5 text-left transition-[background-color,transform] duration-150 hover:bg-surface-2 active:scale-[0.99]"
    >
      <span className="grid size-9 shrink-0 place-items-center rounded-[10px] bg-surface-2 text-ink-2 transition-colors duration-150 group-hover:text-ink">
        <Globe size={16} strokeWidth={1.8} />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block text-[13px] font-medium text-ink">Web preview</span>
        <span className="block truncate text-[12px] text-ink-3">{previewHost(url)}</span>
      </span>
      <span className="shrink-0 rounded-full px-2.5 py-1 text-[12px] font-medium text-ink-3 transition-colors duration-150 group-hover:text-ink">
        Open
      </span>
    </button>
  )
}
