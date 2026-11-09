const providers = ['openai', 'openai-api-key', 'claude'] as const

export type VoiceProvider = typeof providers[number]

export function voiceProvider(value: unknown): VoiceProvider {
  const provider = providers.find((provider) => provider === value)
  if (!provider) {
    throw new Error('Unsupported voice provider. Choose a provider in Voice settings.')
  }
  return provider
}
