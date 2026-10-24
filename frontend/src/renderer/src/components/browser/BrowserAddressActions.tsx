import { ExternalLink, QrCode } from 'lucide-react'
import { useState } from 'react'
import { IconButton } from '@/components/ui/IconButton'
import { Popover } from '@/components/ui/Popover'
import { QRCodeImage } from '@/components/ui/QRCodeImage'
import { openExternal } from '@/lib/clientRuntime'

export function BrowserAddressActions({ url, visible }: { url: string; visible: boolean }) {
  const [open, setOpen] = useState(false)
  const shown = open && visible
  return <div className="-mr-1.5 flex shrink-0 items-center opacity-0 transition-opacity duration-150 group-hover/address:opacity-100 has-focus-visible:opacity-100 has-aria-expanded:opacity-100 pointer-coarse:opacity-100">
    <Popover
      open={shown}
      onClose={() => setOpen(false)}
      placement="below"
      align="end"
      trigger={<IconButton size="2xs" className="pointer-coarse:size-7" aria-label="Open on phone" title="Open on phone" aria-expanded={shown} onClick={() => setOpen((value) => !value)}><QrCode size={12} /></IconButton>}
    >
      <div className="grid size-41 place-items-center rounded-lg bg-white p-1.5">
        <QRCodeImage value={url} label="QR code for this page" />
      </div>
    </Popover>
    <IconButton size="2xs" className="pointer-coarse:size-7" aria-label="Open in Browser" title="Open in Browser" onClick={() => openExternal(url)}><ExternalLink size={12} /></IconButton>
  </div>
}
