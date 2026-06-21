import { useSyncExternalStore } from 'react'
import { setTelemetryEnabled, telemetry, telemetryEnabled } from '@/lib/telemetry'

export function useTelemetryEnabled(): [boolean, (enabled: boolean) => void] {
  const enabled = useSyncExternalStore(telemetry.subscribe, telemetryEnabled, () => false)
  return [enabled, setTelemetryEnabled]
}
