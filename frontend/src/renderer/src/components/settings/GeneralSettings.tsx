import { Switch } from '@/components/ui/Switch'
import { useTelemetryEnabled } from '@/lib/telemetry/useTelemetryEnabled'
import { SettingsCard } from './SettingsCard'
import { useExperimentalFeaturesEnabled } from './sections'

export function GeneralSettings() {
  const [telemetryEnabled, setTelemetryEnabled] = useTelemetryEnabled()
  const [experimentalEnabled, setExperimentalEnabled] = useExperimentalFeaturesEnabled()

  return (
    <section className="py-4">
      <div className="space-y-4">
        <section>
          <div>
            <p className="text-sm font-medium text-ink">Features</p>
            <p className="mt-0.5 text-[13px] text-ink-2">Control which product surfaces appear in Jaz.</p>
          </div>

          <SettingsCard className="mt-4 overflow-hidden">
            <div className="grid grid-cols-1 gap-2 px-3 py-2.5 md:grid-cols-[minmax(0,1fr)_auto] md:items-center">
              <div className="min-w-0">
                <p className="text-[13px] font-medium text-ink">Enable experimental features</p>
                <p className="mt-0.5 text-[12px] text-ink-3">
                  Shows experimental sections in release builds. Development builds always show them.
                </p>
              </div>
              <div className="md:justify-self-end">
                <Switch
                  checked={experimentalEnabled}
                  onChange={setExperimentalEnabled}
                  aria-label="Enable experimental features"
                />
              </div>
            </div>
          </SettingsCard>
        </section>

        <section>
          <div>
            <p className="text-sm font-medium text-ink">Telemetry</p>
            <p className="mt-0.5 text-[13px] text-ink-2">
              Anonymous product telemetry for improving Jaz.
            </p>
          </div>

          <SettingsCard className="mt-4 overflow-hidden">
            <div className="grid grid-cols-1 gap-2 px-3 py-2.5 md:grid-cols-[minmax(0,1fr)_auto] md:items-center">
              <div className="min-w-0">
                <p className="text-[13px] font-medium text-ink">Share telemetry</p>
                <p className="mt-0.5 text-[12px] text-ink-3">
                  Sends coarse action metadata only. No prompts, transcripts, paths, file names,
                  recordings, or page tracking.
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
      </div>
    </section>
  )
}
