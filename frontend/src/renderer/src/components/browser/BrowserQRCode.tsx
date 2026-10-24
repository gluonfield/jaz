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
    <div className="grid size-41 place-items-center rounded-lg bg-white p-1.5">
      <QRCodeImage value={url} alt="QR code for this page" />
    </div>
  </Popover>
}
