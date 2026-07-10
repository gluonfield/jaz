import { describe, expect, test } from 'bun:test'
import { modelProviderModelsRequest } from './modelProviderRequest'

describe('modelProviderModelsRequest', () => {
  test('keeps the selected agent in the request and cache identity', () => {
    const request = modelProviderModelsRequest('openai', 'codex')

    expect(request.path).toBe('/v1/model-providers/openai/models?agent=codex')
    expect(request.queryKey).toEqual(['model-providers', 'openai', 'models', 'codex'])
  })

  test('keeps unscoped provider requests generic', () => {
    const request = modelProviderModelsRequest('openrouter', '')

    expect(request.path).toBe('/v1/model-providers/openrouter/models')
    expect(request.queryKey).toEqual(['model-providers', 'openrouter', 'models'])
  })
})
