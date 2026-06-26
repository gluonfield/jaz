import {
  ArchiveRestore,
  Bot,
  Boxes,
  Brain,
  ChartNoAxesColumn,
  Globe,
  Keyboard,
  Link2,
  type LucideIcon,
  MonitorSmartphone,
  Palette,
  Plug,
  SlidersHorizontal,
  Sparkles,
} from 'lucide-react'

export type SettingsSection =
  | 'general'
  | 'appearance'
  | 'personalization'
  | 'memory'
  | 'connections'
  | 'browser'
  | 'usage'
  | 'devices'
  | 'keyboard'
  | 'mcp'
  | 'providers'
  | 'agents'
  | 'archived'

type SettingsNavItem = {
  id: SettingsSection
  label: string
  icon: LucideIcon
  fullHeight?: boolean
}

export const SETTINGS_SECTIONS: SettingsNavItem[] = [
  { id: 'general', label: 'General', icon: SlidersHorizontal },
  { id: 'appearance', label: 'Appearance', icon: Palette },
  { id: 'personalization', label: 'Personalization', icon: Sparkles, fullHeight: true },
  { id: 'memory', label: 'Memory', icon: Brain },
  { id: 'connections', label: 'Connections', icon: Link2 },
  { id: 'browser', label: 'Browser', icon: Globe },
  { id: 'usage', label: 'Usage', icon: ChartNoAxesColumn },
  { id: 'devices', label: 'Devices', icon: MonitorSmartphone },
  { id: 'keyboard', label: 'Keyboard shortcuts', icon: Keyboard },
  { id: 'mcp', label: 'MCP servers', icon: Plug },
  { id: 'providers', label: 'Model Providers', icon: Boxes },
  { id: 'agents', label: 'Agents (ACP)', icon: Bot },
  { id: 'archived', label: 'Archived threads', icon: ArchiveRestore },
]

export const isSettingsSection = (value: unknown): value is SettingsSection =>
  SETTINGS_SECTIONS.some((item) => item.id === value)
