import { expect, test } from 'bun:test'
import { editTable, lookOf, tableRows, toggleList, toggleMark, toggleMarkAt, toggleTask } from '@/components/canvas/format'

test('an item is plain text, a note or a shape by its colour and shape, and unknown values are ignored', () => {
  expect(lookOf({ id: 'a', type: 'text', size: 'title', ink: 'blue', textAlign: 'right' })).toEqual({ kind: 'text', shape: undefined, fill: undefined, ink: { name: 'blue' }, size: 'title', align: 'right', hollow: false })
  expect(lookOf({ id: 'a', type: 'text', color: '3' })).toMatchObject({ kind: 'note', fill: { name: 'yellow' } })
  expect(lookOf({ id: 'a', type: 'text', color: '#fde2e4', shape: 'cloud', hollow: true })).toMatchObject({ kind: 'block', shape: 'cloud', fill: { custom: '#fde2e4' }, hollow: true })
  expect(lookOf({ id: 'a', type: 'text', shape: 'star', size: 'giant', textAlign: 'justify', hollow: 'yes', color: 7 })).toEqual({ kind: 'text', shape: undefined, fill: undefined, ink: undefined, size: undefined, align: undefined, hollow: false })
})

test('bold and italic mark every written line, keep list markers outside the mark, and come off again', () => {
  const text = 'Plan\n\n- first\n2. second'
  const bold = toggleMark(text, '**')
  expect(bold).toBe('**Plan**\n\n- **first**\n2. **second**')
  expect(toggleMark(bold, '**')).toBe(text)
  const both = toggleMark(bold, '*')
  expect(both).toBe('***Plan***\n\n- ***first***\n2. ***second***')
  expect(toggleMark(both, '**')).toBe('*Plan*\n\n- *first*\n2. *second*')
  expect(toggleMark('**Plan**\nrest', '**')).toBe('**Plan**\n**rest**')
  expect(toggleMark('**Plan**', '*')).toBe('***Plan***')
  expect(toggleMark('', '**')).toBe('')
})

test('a mark wraps the selected words and keeps them selected, or unwraps them', () => {
  const wrapped = toggleMarkAt('ship it now', 5, 7, '**')
  expect(wrapped).toEqual({ text: 'ship **it** now', start: 7, end: 9 })
  expect(toggleMarkAt(wrapped.text, wrapped.start, wrapped.end, '**')).toEqual({ text: 'ship it now', start: 5, end: 7 })
})

test('the list control bullets every written line, replacing numbers, or removes the bullets', () => {
  expect(toggleList('one\n\n2. two\n- three')).toBe('- one\n\n- two\n- three')
  expect(toggleList('- one\n\n* two')).toBe('one\n\ntwo')
})

test('table rows are read as a full grid of strings whatever was stored', () => {
  expect(tableRows({ id: 't', type: 'table', rows: [['a', 2], 'junk', ['b', null, 'c']] })).toEqual([['a', '2', ''], ['b', '', 'c']])
  expect(tableRows({ id: 't', type: 'table' })).toEqual([['']])
})

test('table edits change one cell, add and remove rows and columns, and keep the last row and column', () => {
  const rows = [['a', 'b'], ['c', 'd']]
  expect(editTable(rows, { cell: [1, 0], text: 'x' })).toEqual([['a', 'b'], ['x', 'd']])
  expect(editTable(rows, { addRow: 1 })).toEqual([['a', 'b'], ['', ''], ['c', 'd']])
  expect(editTable(rows, { addColumn: 2 })).toEqual([['a', 'b', ''], ['c', 'd', '']])
  expect(editTable(rows, { removeRow: 0 })).toEqual([['c', 'd']])
  expect(editTable(rows, { removeColumn: 1 })).toEqual([['a'], ['c']])
  expect(editTable([['a']], { removeRow: 0 })).toEqual([['a']])
  expect(editTable([['a']], { removeColumn: 0 })).toEqual([['a']])
})

test('ticking a task changes that task alone', () => {
  const text = '- [ ] one\n  - [x] two\n- not [ ] a task\n* [ ] three'
  expect(toggleTask(text, 1)).toBe('- [ ] one\n  - [ ] two\n- not [ ] a task\n* [ ] three')
  expect(toggleTask(text, 2)).toBe('- [ ] one\n  - [x] two\n- not [ ] a task\n* [x] three')
  expect(toggleTask(text, 3)).toBe(text)
})
