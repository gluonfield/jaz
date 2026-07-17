import { describe, expect, test } from 'bun:test'
import { filterModelSuggestions, modelSuggestionLabel } from './modelSuggestion'
import { effectiveReasoningEffort, modelReasoningSelection } from './reasoningEfforts'

const input = {
  settings: undefined,
  agent: 'codex',
  model: 'gpt-test',
  requested: 'high',
  settingsMode: false,
}

describe('modelReasoningSelection', () => {
  test.each(['pending', 'error'])('preserves effort while the catalog is %s', (status) => {
    const selection = modelReasoningSelection({ ...input, catalog: { status } })

    expect(selection).toEqual({
      options: [],
      effectiveEffort: 'high',
      supported: true,
      status,
      blocked: true,
    })
  })

  test('clears effort only after an explicit unavailable capability', () => {
    const selection = modelReasoningSelection({
      ...input,
      catalog: {
        status: 'ready',
        unknownModel: 'unavailable',
        suggestions: [{ value: 'gpt-test', label: 'Test', reasoning: { status: 'unavailable' } }],
      },
    })

    expect(selection.effectiveEffort).toBe('')
    expect(selection.supported).toBeFalse()
    expect(selection.status).toBe('ready')
    expect(selection.blocked).toBeFalse()
  })

  test('uses advertised model efforts when capabilities are ready', () => {
    const catalog = {
      status: 'ready',
      unknownModel: 'unavailable',
      suggestions: [{
        value: 'gpt-test',
        label: 'Test',
        reasoning: { status: 'ready', efforts: ['low', 'high'] },
      }],
    }

    expect(modelReasoningSelection({ ...input, catalog }).supported).toBeTrue()
    expect(modelReasoningSelection({ ...input, requested: 'minimal', catalog }).supported).toBeFalse()
  })

  test('uses server-provided aliases across providers', () => {
    const suggestions = [{
      value: 'openai/gpt-test',
      label: 'Test',
      aliases: ['gpt-test'],
      reasoning: { status: 'ready', efforts: ['high'] },
    }]
    const selection = modelReasoningSelection({
      ...input,
      catalog: {
        status: 'ready',
        unknownModel: 'unavailable',
        suggestions,
      },
    })

    expect(selection.supported).toBeTrue()
    expect(selection.effectiveEffort).toBe('high')
    expect(modelSuggestionLabel(suggestions, 'gpt-test')).toBe('Test')
    expect(filterModelSuggestions(suggestions, 'gpt-test')).toEqual(suggestions)
  })

  test('preserves effort when a ready catalog still marks the model pending', () => {
    const selection = modelReasoningSelection({
      ...input,
      catalog: {
        status: 'ready',
        unknownModel: 'unavailable',
        suggestions: [{ value: 'gpt-test', label: 'Test', reasoning: { status: 'pending' } }],
      },
    })

    expect(selection.effectiveEffort).toBe('high')
    expect(selection.supported).toBeTrue()
    expect(selection.status).toBe('pending')
    expect(selection.blocked).toBeTrue()
  })
})

describe('effectiveReasoningEffort', () => {
  // Provider models (e.g. OpenRouter kimi-k3) expose Default + their efforts, never a
  // 'none' sentinel. A stale inherited effort the model does not support must clamp to
  // '' — this is the single value the composer both shows and launches, so a leftover
  // 'xhigh' never reaches the backend as an unsupported reasoning_effort.
  const providerOnlyMax = [
    { value: '', label: 'Default' },
    { value: 'max', label: 'Max' },
  ]

  test('clamps an unsupported effort to Default when no none option exists', () => {
    expect(effectiveReasoningEffort('xhigh', providerOnlyMax)).toBe('')
  })

  test('passes a supported effort through unchanged', () => {
    expect(effectiveReasoningEffort('max', providerOnlyMax)).toBe('max')
    expect(effectiveReasoningEffort('', providerOnlyMax)).toBe('')
  })

  test('clamps to none when the model offers an explicit none option', () => {
    expect(effectiveReasoningEffort('xhigh', [{ value: 'none', label: 'None' }])).toBe('none')
  })
})
