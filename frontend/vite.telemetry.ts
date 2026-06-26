import { loadEnv } from 'vite'

export function defineTelemetryEnv(mode: string): Record<string, string> {
  const env = loadEnv(mode, process.cwd(), '')
  return {
    'import.meta.env.VITE_POSTHOG_TOKEN': JSON.stringify(
      envValue(env.VITE_POSTHOG_TOKEN) ?? envValue(env.POSTHOG_PROJECT_TOKEN) ?? '',
    ),
  }
}

function envValue(value: string | undefined): string | undefined {
  const trimmed = value?.trim()
  return trimmed === '' ? undefined : trimmed
}
