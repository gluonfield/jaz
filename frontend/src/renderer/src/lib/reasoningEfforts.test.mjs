import { describe, expect, test } from 'bun:test'
import { filterModelSuggestions, modelSuggestionLabel } from './modelSuggestion'
import { effectiveReasoningEffort, modelReasoningSelection, sessionReasoningEffortOptions } from './reasoningEfforts'

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

  test('suppresses the inherited effort after an explicit unavailable capability', () => {
    const selection = modelReasoningSelection({
      ...input,
      catalog: {
        status: 'ready',
        unknownModel: 'unavailable',
        suggestions: [{ value: 'gpt-test', label: 'Test', reasoning: { status: 'unavailable' } }],
      },
    })

    expect(selection.effectiveEffort).toBe('none')
    expect(selection.supported).toBeFalse()
    expect(selection.status).toBe('ready')
    expect(selection.blocked).toBeFalse()
  })

  test('does not infer OpenCode model controls from the agent-wide effort list', () => {
    const settings = {
      acp_options: {
        opencode: {
          reasoning_efforts: [
            { value: '', label: 'Default' },
            { value: 'low', label: 'Low' },
            { value: 'high', label: 'High' },
            { value: 'max', label: 'Max' },
          ],
        },
      },
    }
    const catalog = {
      status: 'ready',
      unknownModel: 'unavailable',
      suggestions: [{ value: 'glm-5.3-flash', label: 'GLM Flash', reasoning: { status: 'unavailable' } }],
    }
    const selection = modelReasoningSelection({
      ...input,
      settings,
      agent: 'opencode',
      provider: 'opencode-native',
      model: 'glm-5.3-flash',
      catalog,
    })

    expect(selection.options).toEqual([])
    expect(selection.supported).toBeFalse()
    expect(selection.effectiveEffort).toBe('none')
  })

  test('does not infer OpenCode controls for automatic reasoning', () => {
    const settings = {
      acp_options: { opencode: { reasoning_efforts: [{ value: '', label: 'Default' }, { value: 'high', label: 'High' }] } },
    }
    const selection = modelReasoningSelection({
      ...input,
      settings,
      agent: 'opencode',
      provider: 'opencode-native',
      model: 'glm-5.3-flash',
      catalog: {
        status: 'ready',
        unknownModel: 'unavailable',
        suggestions: [{ value: 'glm-5.3-flash', label: 'GLM Flash', reasoning: { status: 'ready', automatic: true } }],
      },
    })

    expect(selection.options).toEqual([])
    expect(selection.supported).toBeFalse()
  })

  test.each(['pending', 'error'])('keeps unavailable OpenCode catalog state as %s', (status) => {
    expect(modelReasoningSelection({ ...input, agent: 'opencode', catalog: { status } })).toEqual({
      options: [], effectiveEffort: 'high', supported: true, status, blocked: true,
    })
    expect(modelReasoningSelection({ ...input, agent: 'opencode', requested: 'none', catalog: { status } }).blocked).toBeFalse()
  })

  test('keeps an unreported OpenCode model unknown and never invents effort choices', () => {
    const catalog = { status: 'ready', unknownModel: 'ready', suggestions: [] }
    const selection = modelReasoningSelection({ ...input, agent: 'opencode', catalog })
    expect(selection.status).toBe('unknown')
    expect(selection.options).toEqual([])
    expect(selection.effectiveEffort).toBe('high')
    expect(selection.blocked).toBeFalse()
    expect(modelReasoningSelection({ ...input, agent: 'opencode', requested: '', catalog }).blocked).toBeFalse()
  })

  test('uses the current session advertised choices even before catalog discovery', () => {
    const advertisedOptions = sessionReasoningEffortOptions([{
      id: 'variant', category: 'thought_level', current_value: 'none',
      options: [{ value: 'default', name: 'Provider default' }, { value: 'none', name: 'Disabled' }, { value: 'deep', name: 'Deep' }],
    }])
    const selection = modelReasoningSelection({ ...input, agent: 'opencode', requested: 'deep', advertisedOptions, catalog: { status: 'pending' } })
    expect(selection.options.map(({ value }) => value)).toEqual(['default', 'none', 'deep'])
    expect(selection.effectiveEffort).toBe('deep')
    expect(selection.status).toBe('ready')
    expect(selection.blocked).toBeFalse()
    for (const requested of ['none', 'default']) {
      expect(modelReasoningSelection({ ...input, agent: 'opencode', requested, advertisedOptions, catalog: { status: 'error' } }).effectiveEffort).toBe(requested)
    }
    expect(modelReasoningSelection({ ...input, agent: 'opencode', requested: 'high', advertisedOptions, catalog: { status: 'pending' } }).supported).toBeFalse()
    expect(sessionReasoningEffortOptions([])).toBeUndefined()
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

  test('lets a thinking model without effort controls use its automatic default', () => {
    const catalog = {
      status: 'ready',
      unknownModel: 'unavailable',
      suggestions: [{ value: 'gpt-test', label: 'Test', reasoning: { status: 'ready', automatic: true } }],
    }
    const selection = modelReasoningSelection({
      ...input,
      catalog,
    })

    expect(selection.options).toEqual([])
    expect(selection.effectiveEffort).toBe('none')
    expect(selection.supported).toBeFalse()
    expect(selection.blocked).toBeFalse()

    expect(modelReasoningSelection({ ...input, requested: 'none', catalog }).supported).toBeTrue()
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

  test('clears an inherited effort that the selected model cannot accept', () => {
    expect(effectiveReasoningEffort('xhigh', providerOnlyMax)).toBe('none')
  })

  test('passes a supported effort through unchanged', () => {
    expect(effectiveReasoningEffort('max', providerOnlyMax)).toBe('max')
    expect(effectiveReasoningEffort('', providerOnlyMax)).toBe('')
  })

  test('preserves the explicit clear sentinel regardless of provider options', () => {
    expect(effectiveReasoningEffort('none', providerOnlyMax)).toBe('none')
    expect(effectiveReasoningEffort('xhigh', [{ value: 'none', label: 'None' }])).toBe('none')
  })

  test('clamps to none when reasoning is automatic or unavailable', () => {
    expect(effectiveReasoningEffort('xhigh', [])).toBe('none')
  })
})
