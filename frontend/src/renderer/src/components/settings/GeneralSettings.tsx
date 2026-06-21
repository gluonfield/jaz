import { Switch } from '@/components/ui/Switch'
import { useTelemetryEnabled } from '@/lib/telemetry/useTelemetryEnabled'
import { SettingsCard } from './SettingsCard'
import { ThemeSwitcher } from './ThemeSwitcher'

export function GeneralSettings() {
  const [telemetryEnabled, setTelemetryEnabled] = useTelemetryEnabled()

  return (
    <section className="py-5">
      <div>
        <p className="text-sm font-medium text-ink">Appearance</p>
        <p className="mt-0.5 text-[13px] text-ink-2">How the interface looks.</p>
      </div>

      <SettingsCard className="mt-4 overflow-hidden">
        <div className="grid grid-cols-1 gap-2 px-3 py-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-center">
          <div className="min-w-0">
            <p className="text-[13px] font-medium text-ink">Theme</p>
            <p className="mt-0.5 text-[12px] text-ink-3">Match the system, or pick light or dark.</p>
          </div>
          <div className="md:justify-self-end">
            <ThemeSwitcher />
          </div>
        </div>
      </SettingsCard>

      <div className="mt-8">
        <p className="text-sm font-medium text-ink">Telemetry</p>
        <p className="mt-0.5 text-[13px] text-ink-2">Anonymous product telemetry for improving Jaz.</p>
      </div>

      <SettingsCard className="mt-4 overflow-hidden">
        <div className="grid grid-cols-1 gap-2 px-3 py-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-center">
          <div className="min-w-0">
            <p className="text-[13px] font-medium text-ink">Share telemetry</p>
            <p className="mt-0.5 text-[12px] text-ink-3">
              Sends coarse action metadata only. No prompts, transcripts, paths, file names, recordings, or page
              tracking.
            </p>
          </div>
          <div className="md:justify-self-end">
            <Switch
              checked={telemetryEnabled}
              onChange={setTelemetryEnabled}
              aria-label="Share telemetry"
            />
          </div>
        </div>
      </SettingsCard>
    </section>
  )
}
