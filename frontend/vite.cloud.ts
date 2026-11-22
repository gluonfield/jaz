import { loadEnv } from 'vite'

export function defineCloudEnv(mode: string): Record<string, string> {
  const env = loadEnv(mode, process.cwd(), '')
  const cloudUrl = envValue(env.JAZ_CLOUD_URL) ?? envValue(env.VITE_JAZ_CLOUD_URL) ?? ''
  const localEnabled = envValue(env.JAZ_LOCAL_ENABLED) ?? envValue(env.VITE_JAZ_LOCAL_ENABLED) ?? ''
  return {
    'import.meta.env.VITE_JAZ_CLOUD_URL': JSON.stringify(cloudUrl),
    'import.meta.env.VITE_JAZ_LOCAL_ENABLED': JSON.stringify(localEnabled),
    __JAZ_BUILD_CLOUD_URL__: JSON.stringify(cloudUrl),
    __JAZ_BUILD_LOCAL_ENABLED__: JSON.stringify(localEnabled),
  }
}

function envValue(value: string | undefined): string | undefined {
  const trimmed = value?.trim()
  return trimmed === '' ? undefined : trimmed
}
