import { QrCode } from 'lucide-react'
import { useState } from 'react'
import { IconButton } from '@/components/ui/IconButton'
import { Popover } from '@/components/ui/Popover'
import { QRCodeImage } from '@/components/ui/QRCodeImage'

export function BrowserQRCode({ url, visible }: { url: string; visible: boolean }) {
  const [open, setOpen] = useState(false)
  const shown = open && visible && Boolean(url)
  return <Popover
    open={shown}
    onClose={() => setOpen(false)}
    placement="below"
    align="end"
    trigger={<IconButton
      aria-label="Open on phone"
      title="Open on phone"
      aria-expanded={shown}
      disabled={!url}
      onClick={() => setOpen((value) => !value)}
      size="sm"
      className="pointer-coarse:size-10"
    ><QrCode size={14} /></IconButton>}
  >
    <div className="w-52 p-1">
      <div className="grid aspect-square place-items-center rounded-[10px] bg-white p-2 shadow-[inset_0_0_0_1px_rgba(0,0,0,0.1)]">
        <QRCodeImage value={url} alt="QR code for this page" />
      </div>
      <p className="mt-2 line-clamp-2 break-all px-0.5 text-[11px] text-ink-3">{url}</p>
    </div>
  </Popover>
}
