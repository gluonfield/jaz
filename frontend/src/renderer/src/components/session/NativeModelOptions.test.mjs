import { expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { NativeModelOptions } from '@/components/session/NativeModelOptions'

const options = [
  {
    id: 'fast-mode', name: 'Fast mode', category: 'model_config', current_value: 'on',
    description: '1.5x speed, increased usage',
    options: [{ value: 'off', name: 'Off' }, { value: 'on', name: 'On' }],
  },
]

test('the running composer keeps Fast Mode available without fixed speed or usage claims', () => {
  const html = renderToStaticMarkup(createElement(NativeModelOptions, {
    options, running: true, pending: false, onChange() {},
  }))
  const buttons = html.match(/<button\b[^>]*>/g)
  const fast = buttons.find((button) => button.includes('role="switch"'))
  expect(fast).toContain('aria-label="Fast Mode"')
  expect(fast).toContain('aria-checked="true"')
  expect(fast).not.toMatch(/\sdisabled(?:=|>)/)
  expect(html).not.toContain('1.5x')
  expect(html).not.toContain('increased usage')
})

test('Fast Mode waits for acknowledgement and disappears when the provider withdraws it', () => {
  const pending = renderToStaticMarkup(createElement(NativeModelOptions, {
    options, running: true, pending: true, onChange() {},
  }))
  expect(pending.match(/<button\b[^>]*role="switch"[^>]*>/)[0]).toMatch(/\sdisabled(?:=|>)/)
  const withdrawn = renderToStaticMarkup(createElement(NativeModelOptions, {
    options: options.slice(1), running: true, pending: false, onChange() {},
  }))
  expect(withdrawn).not.toContain('Fast Mode')
  expect(withdrawn).not.toContain('role="switch"')
})

test('native model options are exposed only when explicitly enabled', () => {
  const model = {
    id: 'model', name: 'OpenCode model', category: 'model', current_value: 'opencode-go/glm',
    options: [{ value: 'opencode-go/glm', name: 'OpenCode Go / GLM' }, { value: 'zai-coding-plan/glm', name: 'z.ai / GLM' }],
  }
  const props = { options: [model], running: false, pending: false, onChange() {} }
  expect(NativeModelOptions(props)).toHaveLength(0)
  const shown = NativeModelOptions({ ...props, showAgentModels: true })
  expect(shown).toHaveLength(1)
  expect(shown[0].key).toBe('model')
})

test('the session exposes its advertised reasoning variants without inventing generic levels', () => {
  const reasoning = {
    id: 'variant', name: 'Reasoning variant', category: 'thought_level', current_value: 'deep',
    options: [{ value: 'default', name: 'Model default' }, { value: 'deep', name: 'Deep reasoning' }],
  }
  const changes = []
  const rendered = NativeModelOptions({
    options: [reasoning], running: false, pending: false, onChange: (...change) => changes.push(change),
  })
  const control = rendered[0].props.children[0].props.children[1]
  expect(control.props.value).toBe('deep')
  expect(control.props.options).toEqual([{ value: 'default', label: 'Model default' }, { value: 'deep', label: 'Deep reasoning' }])
  control.props.onChange('default')
  expect(changes).toEqual([['variant', 'default']])
})
