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
import { useSyncExternalStore } from 'react'

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
  experimental?: boolean
}

export const SETTINGS_SECTIONS: SettingsNavItem[] = [
  { id: 'general', label: 'General', icon: SlidersHorizontal },
  { id: 'appearance', label: 'Appearance', icon: Palette },
  { id: 'personalization', label: 'Personalization', icon: Sparkles, fullHeight: true },
  { id: 'memory', label: 'Memory', icon: Brain },
  { id: 'connections', label: 'Connections', icon: Link2, experimental: true },
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

const EXPERIMENTAL_FEATURES_ENABLED_KEY = 'jaz.experimentalFeatures.enabled'
const EXPERIMENTAL_FEATURES_EVENT = 'jaz:experimental-features'

export function useExperimentalFeaturesEnabled(): [boolean, (enabled: boolean) => void] {
  const enabled = useSyncExternalStore(subscribe, experimentalFeaturesEnabled, () => false)
  return [enabled, setExperimentalFeaturesEnabled]
}

export function visibleSettingsSections(experimentalEnabled: boolean): SettingsNavItem[] {
  if (import.meta.env.DEV || experimentalEnabled) return SETTINGS_SECTIONS
  return SETTINGS_SECTIONS.filter((item) => !item.experimental)
}

function experimentalFeaturesEnabled(): boolean {
  if (typeof window === 'undefined') return false
  try {
    return window.localStorage.getItem(EXPERIMENTAL_FEATURES_ENABLED_KEY) === 'true'
  } catch {
    return false
  }
}

function setExperimentalFeaturesEnabled(enabled: boolean) {
  if (typeof window === 'undefined') return
  try {
    if (enabled) window.localStorage.setItem(EXPERIMENTAL_FEATURES_ENABLED_KEY, 'true')
    else window.localStorage.removeItem(EXPERIMENTAL_FEATURES_ENABLED_KEY)
  } catch {
    return
  }
  window.dispatchEvent(new Event(EXPERIMENTAL_FEATURES_EVENT))
}

function subscribe(callback: () => void): () => void {
  const listener = () => callback()
  window.addEventListener(EXPERIMENTAL_FEATURES_EVENT, listener)
  window.addEventListener('storage', listener)
  return () => {
    window.removeEventListener(EXPERIMENTAL_FEATURES_EVENT, listener)
    window.removeEventListener('storage', listener)
  }
}
