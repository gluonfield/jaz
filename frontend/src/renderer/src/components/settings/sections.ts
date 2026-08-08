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
const DEFAULT_EXPERIMENTAL_FEATURES_ENABLED = true

export function useExperimentalFeaturesEnabled(): [boolean, (enabled: boolean) => void] {
  const enabled = useSyncExternalStore(
    subscribe,
    experimentalFeaturesEnabled,
    () => DEFAULT_EXPERIMENTAL_FEATURES_ENABLED,
  )
  return [enabled, setExperimentalFeaturesEnabled]
}

export function visibleSettingsSections(experimentalEnabled: boolean): SettingsNavItem[] {
  if (import.meta.env.DEV || experimentalEnabled) return SETTINGS_SECTIONS
  return SETTINGS_SECTIONS.filter((item) => !item.experimental)
}

function experimentalFeaturesEnabled(): boolean {
  if (typeof window === 'undefined') return DEFAULT_EXPERIMENTAL_FEATURES_ENABLED
  try {
    const stored = window.localStorage.getItem(EXPERIMENTAL_FEATURES_ENABLED_KEY)
    if (stored === null) return DEFAULT_EXPERIMENTAL_FEATURES_ENABLED
    return stored === 'true'
  } catch {
    return DEFAULT_EXPERIMENTAL_FEATURES_ENABLED
  }
}

function setExperimentalFeaturesEnabled(enabled: boolean) {
  if (typeof window === 'undefined') return
  try {
    if (enabled === DEFAULT_EXPERIMENTAL_FEATURES_ENABLED) {
      window.localStorage.removeItem(EXPERIMENTAL_FEATURES_ENABLED_KEY)
    } else {
      window.localStorage.setItem(EXPERIMENTAL_FEATURES_ENABLED_KEY, String(enabled))
    }
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
