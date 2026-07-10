import { keys } from './query/keys'

export function modelProviderModelsRequest(provider: string | undefined, agent: string) {
  const id = provider ?? ''
  const query = agent ? `?agent=${encodeURIComponent(agent)}` : ''
  return {
    queryKey: keys.modelProviderModels(id, agent),
    path: `/v1/model-providers/${encodeURIComponent(id)}/models${query}`,
  }
}
