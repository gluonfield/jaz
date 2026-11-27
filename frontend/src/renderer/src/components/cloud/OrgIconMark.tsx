import {
  Box,
  Building2,
  Compass,
  Factory,
  FlaskConical,
  Globe,
  Leaf,
  type LucideIcon,
  Rocket,
  Shield,
  Sparkles,
  Star,
  Zap,
} from 'lucide-react'
import { apiEmbeddedGetUrl } from '@/lib/api/client'
import type { OrgIcon } from '@/lib/api/serverTypes'

// Preset slugs are stored by Jaz Cloud; a slug this client does not know falls back to the initial.
export const ORG_ICON_PRESETS: Record<string, LucideIcon> = {
  building: Building2,
  rocket: Rocket,
  factory: Factory,
  flask: FlaskConical,
  globe: Globe,
  leaf: Leaf,
  bolt: Zap,
  star: Star,
  shield: Shield,
  cube: Box,
  compass: Compass,
  sparkles: Sparkles,
}

// `className` sets the size, corner radius and initial's font size.
export function OrgIconMark({ name, icon, className }: { name: string; icon?: OrgIcon; className: string }) {
  if (icon?.kind === 'image' && icon.url) {
    return <img src={apiEmbeddedGetUrl(icon.url)} alt="" className={`shrink-0 object-cover ${className}`} />
  }
  const Preset = icon?.kind === 'preset' && icon.preset ? ORG_ICON_PRESETS[icon.preset] : undefined
  return (
    <span className={`grid shrink-0 place-items-center bg-ink font-semibold text-bg uppercase ${className}`}>
      {Preset ? <Preset className="size-[60%]" /> : name.charAt(0)}
    </span>
  )
}
