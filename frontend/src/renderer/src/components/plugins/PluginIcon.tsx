import { PanelsTopLeft } from 'lucide-react'
import { DynamicIcon, iconNames, type IconName } from 'lucide-react/dynamic'

export function PluginIcon({ icon }: { icon: string }) {
  const name = icon.replace(/([a-z0-9])([A-Z])/g, '$1-$2').toLowerCase()
  return iconNames.includes(name as IconName)
    ? <DynamicIcon name={name as IconName} aria-hidden fallback={() => <PanelsTopLeft aria-hidden />} />
    : <PanelsTopLeft aria-hidden />
}
