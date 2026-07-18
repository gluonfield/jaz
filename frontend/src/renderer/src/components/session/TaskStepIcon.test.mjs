import { expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { TaskStepIcon } from './TaskStepIcon'

function renderIcon(state, animate = false) {
  return renderToStaticMarkup(createElement(TaskStepIcon, { state, animate }))
}

test('completed tasks are distinct from pending tasks', () => {
  expect(renderIcon('completed')).toContain('lucide-circle-check')
  expect(renderIcon('pending')).toContain('lucide-circle')
  expect(renderIcon('pending')).not.toContain('lucide-circle-check')
})

test('active tasks spin only while work is active', () => {
  expect(renderIcon('active', true)).toContain('animate-spin')
  expect(renderIcon('active')).not.toContain('animate-spin')
})
