import {
  ArchiveRestore,
  AudioLines,
  Bell,
  Bot,
  Boxes,
  Brain,
  ChartNoAxesColumn,
  Globe,
  Keyboard,
  Laptop,
  Layers,
  type LucideIcon,
  MonitorSmartphone,
  Palette,
  Plug,
  Puzzle,
  ShieldCheck,
  SlidersHorizontal,
  Sparkles,
} from 'lucide-react'
import { useSyncExternalStore } from 'react'

export type SettingsSection =
  | 'general'
  | 'notifications'
  | 'appearance'
  | 'personalization'
  | 'memory'
  | 'browser'
  | 'computer'
  | 'machines'
  | 'cloud'
  | 'admin'
  | 'usage'
  | 'keyboard'
  | 'mcp'
  | 'plugins'
  | 'providers'
  | 'agents'
  | 'voice'
  | 'archived'

export type SettingsGroup = 'workspace' | 'app'

export type SettingsNavItem = {
  id: SettingsSection
  label: string
  icon: LucideIcon
  group: SettingsGroup
  fullHeight?: boolean
  wide?: boolean
  experimental?: boolean
  requiresCloud?: boolean
  requiresCloudAdmin?: boolean
  requiresLocal?: boolean
  requiresLocalOrCloud?: boolean
}

export const SETTINGS_SECTIONS: SettingsNavItem[] = [
  { id: 'cloud', label: 'Workspace', icon: Layers, group: 'workspace' },
  { id: 'admin', label: 'Admin', icon: ShieldCheck, group: 'workspace', wide: true, requiresCloudAdmin: true },
  { id: 'personalization', label: 'Personalization', icon: Sparkles, group: 'workspace', fullHeight: true },
  { id: 'memory', label: 'Memory', icon: Brain, group: 'workspace', requiresLocal: true },
  { id: 'agents', label: 'Agents (ACP)', icon: Bot, group: 'workspace', requiresLocalOrCloud: true },
  { id: 'plugins', label: 'Plugins', icon: Puzzle, group: 'workspace', wide: true },
  { id: 'mcp', label: 'MCP servers', icon: Plug, group: 'workspace' },
  { id: 'machines', label: 'Connected Machines', icon: Laptop, group: 'workspace', requiresCloud: true },
  { id: 'usage', label: 'Usage', icon: ChartNoAxesColumn, group: 'workspace' },
  { id: 'archived', label: 'Archived threads', icon: ArchiveRestore, group: 'workspace' },
  { id: 'general', label: 'General', icon: SlidersHorizontal, group: 'app' },
  { id: 'appearance', label: 'Appearance', icon: Palette, group: 'app' },
  { id: 'notifications', label: 'Notifications', icon: Bell, group: 'app' },
  { id: 'keyboard', label: 'Keyboard shortcuts', icon: Keyboard, group: 'app' },
  { id: 'voice', label: 'Voice', icon: AudioLines, group: 'app', requiresLocal: true },
  { id: 'providers', label: 'Model Providers', icon: Boxes, group: 'app', requiresLocal: true },
  { id: 'browser', label: 'Browser', icon: Globe, group: 'app', requiresLocal: true },
  { id: 'computer', label: 'Computer Use', icon: MonitorSmartphone, group: 'app', requiresLocal: true },
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

export function visibleSettingsSections(
  experimentalEnabled: boolean,
  hasCloud = false,
  localBackend = true,
  cloudAdmin = false,
): SettingsNavItem[] {
  return SETTINGS_SECTIONS.filter((item) => {
    if (item.requiresCloud && !hasCloud) return false
    if (item.requiresCloudAdmin && !cloudAdmin) return false
    if (item.requiresLocal && !localBackend) return false
    if (item.id === 'memory' && hasCloud) return false
    if (item.requiresLocalOrCloud && !localBackend && !hasCloud) return false
    if (!import.meta.env?.DEV && !experimentalEnabled && item.experimental) return false
    return true
  })
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
