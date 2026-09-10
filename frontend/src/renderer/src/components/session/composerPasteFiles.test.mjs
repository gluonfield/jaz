import { expect, test } from 'bun:test'
import { composerPasteFiles } from '@/components/session/composerPasteFiles'

function clipboard(text, files = [], html = '') {
  return {
    items: files.map((file) => ({
      kind: 'file',
      type: file.type,
      getAsFile: () => file,
    })),
    files,
    getData: (type) => type === 'text/plain' ? text : html,
  }
}

test('pastes through 2,000 Unicode characters stay in the text field', () => {
  for (const text of ['', 'short paste', 'a'.repeat(2000), '😀'.repeat(2000)]) {
    expect(composerPasteFiles(clipboard(text))).toEqual([])
  }
})

test('larger pastes become UTF-8 text files with every character preserved', async () => {
  for (const text of ['a'.repeat(2001), '😀'.repeat(2001), `  ${'α\tβ\r\n'.repeat(20000)}\n  `]) {
    const files = composerPasteFiles(clipboard(text, [], '<p>Rich clipboard formatting</p>'))
    expect(files).toHaveLength(1)
    expect(files[0].name).toMatch(/^pasted-text-\d+\.txt$/)
    expect(files[0].type.split(';')[0]).toBe('text/plain')
    expect(await files[0].text()).toBe(text)
    expect(files[0].size).toBe(new TextEncoder().encode(text).length)
  }
})

test('repeated pastes have distinct attachment names and retain their own contents', async () => {
  const firstText = 'a'.repeat(2001)
  const secondText = 'b'.repeat(2001)
  const [first] = composerPasteFiles(clipboard(firstText))
  const [second] = composerPasteFiles(clipboard(secondText))
  expect(first.name).not.toBe(second.name)
  expect(await first.text()).toBe(firstText)
  expect(await second.text()).toBe(secondText)
})

test('clipboard images and files take precedence over accompanying text', () => {
  const files = [
    new File(['image'], 'screenshot.png', { type: 'image/png' }),
    new File(['document'], 'notes.pdf', { type: 'application/pdf' }),
  ]
  for (const itemsAvailable of [true, false]) {
    const data = clipboard('a'.repeat(2001), files)
    if (!itemsAvailable) {
      data.items = []
    }
    const result = composerPasteFiles(data)
    expect(result).toHaveLength(2)
    expect(result[0]).toBe(files[0])
    expect(result[1]).toBe(files[1])
  }
})

test('HTML without plain text stays with the browser paste handler', () => {
  expect(composerPasteFiles(clipboard('', [], `<p>${'a'.repeat(2001)}</p>`))).toEqual([])
})
