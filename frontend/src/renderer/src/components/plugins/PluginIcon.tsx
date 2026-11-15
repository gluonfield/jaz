import { PanelsTopLeft } from 'lucide-react'

export function PluginIcon({ icon }: { icon: string }) {
  if (!icon) return <PanelsTopLeft aria-hidden />
  return <span aria-hidden className="size-[18px] shrink-0 bg-current" style={{ maskImage: `url("${icon}")`, maskSize: 'contain', maskRepeat: 'no-repeat', maskPosition: 'center' }} />
}
