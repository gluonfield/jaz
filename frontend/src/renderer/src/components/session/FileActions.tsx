import { useQuery } from '@tanstack/react-query'
import { ChevronDown, Copy, Download, FolderOpen } from 'lucide-react'
import { useState } from 'react'
import { IconButton } from '@/components/ui/IconButton'
import { MenuRow, Popover } from '@/components/ui/Popover'
import { useToast } from '@/components/ui/toast'
import { apiBaseUrl } from '@/lib/api/client'
import { sessionFileDownloadUrl } from '@/lib/api/sessions'
import { writeClipboard } from '@/lib/clipboard'

export function CopyPathMenuRow({ path, onClose, autoFocus }: { path: string; onClose: () => void; autoFocus?: boolean }) {
  const toast = useToast()
  return <MenuRow role="menuitem" autoFocus={autoFocus} onClick={async () => {
    onClose()
    if (await writeClipboard(path)) toast('Path copied')
    else toast('Could not copy path', 'danger')
  }}>
    <span className="flex items-center gap-2"><Copy size={15} aria-hidden />Copy path</span>
  </MenuRow>
}

export function FileActions({ sessionId, path, beforeDownload }: { sessionId: string; path: string; beforeDownload?: () => Promise<boolean> }) {
  const toast = useToast()
  const [open, setOpen] = useState(false)
  const backendUrl = apiBaseUrl()
  const href = sessionFileDownloadUrl(sessionId, path)
  const { data: canReveal } = useQuery({
    queryKey: ['fileFolder', backendUrl, sessionId],
    queryFn: () => window.jaz?.fileActions?.canReveal(backendUrl, sessionId) ?? Promise.resolve(false),
    staleTime: 60_000,
    retry: false,
  })
  const download = async () => {
    setOpen(false)
    if (beforeDownload && !await beforeDownload()) return
    const link = document.createElement('a')
    link.href = href
    link.download = ''
    link.click()
  }
  return <Popover open={open} onClose={() => setOpen(false)} placement="below" align="end"
    trigger={<IconButton size="sm" aria-label="File actions" title="File actions" aria-haspopup="menu" aria-expanded={open} onClick={() => setOpen(!open)}>
      <ChevronDown size={15} />
    </IconButton>}>
    <div role="menu" aria-label="File actions">
      <CopyPathMenuRow path={path} onClose={() => setOpen(false)} autoFocus />
      {canReveal && (
        <MenuRow role="menuitem" onClick={() => {
          setOpen(false)
          void window.jaz?.fileActions.reveal(backendUrl, sessionId, path).catch((error: Error) => toast(error.message, 'danger'))
        }}>
          <span className="flex items-center gap-2"><FolderOpen size={15} aria-hidden />Open folder</span>
        </MenuRow>
      )}
      <MenuRow role="menuitem" onClick={() => void download()}>
        <span className="flex items-center gap-2"><Download size={15} aria-hidden />Download file</span>
      </MenuRow>
    </div>
  </Popover>
}
