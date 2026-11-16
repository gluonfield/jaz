import { useQuery } from '@tanstack/react-query'
import { Download, FolderOpen } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { useToast } from '@/components/ui/toast'
import { apiBaseUrl } from '@/lib/api/client'
import { sessionFileDownloadUrl } from '@/lib/api/sessions'

export function FileActions({ sessionId, path, beforeDownload }: { sessionId: string; path: string; beforeDownload?: () => Promise<boolean> }) {
  const toast = useToast()
  const local = useQuery({
    queryKey: ['fileFolder', apiBaseUrl(), sessionId],
    queryFn: () => window.jaz?.fileActions?.canReveal(sessionId) ?? Promise.resolve(false),
    staleTime: 60_000,
    retry: false,
  })
  return <div className="flex shrink-0 items-center gap-1">
    <Button size="sm" className="h-10 min-w-10 px-2" whileTap={{ scale: 0.96 }} disabled={!local.data}
      aria-label="Open folder" title={local.data ? 'Open folder' : 'Available in Jaz desktop when connected to the server on this machine'}
      onClick={() => void window.jaz?.fileActions.reveal(sessionId, path).catch((error: Error) => toast(error.message, 'danger'))}>
      <FolderOpen size={15} aria-hidden /><span className="hidden @min-[540px]/file:inline">Open folder</span>
    </Button>
    <a href={sessionFileDownloadUrl(sessionId, path)} download aria-label="Download file" title="Download file"
      onClick={(event) => {
        if (!beforeDownload) return
        event.preventDefault()
        const href = event.currentTarget.href
        void beforeDownload().then((ready) => {
          if (!ready) return
          const link = document.createElement('a')
          link.href = href
          link.download = ''
          link.click()
        })
      }}
      className="inline-flex h-10 min-w-10 shrink-0 items-center justify-center gap-1.5 rounded-full px-2 text-[12px] font-medium text-ink-2 transition-[background-color,color,transform] hover:bg-surface-2 hover:text-ink focus-visible:outline-2 focus-visible:outline-primary active:scale-[0.96]">
      <Download size={15} aria-hidden /><span className="hidden @min-[540px]/file:inline">Download file</span>
    </a>
  </div>
}
