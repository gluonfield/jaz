import { describe, expect, test } from 'bun:test'
import { modelReasoningSelection } from './reasoningEfforts'

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
