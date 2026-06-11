interface Shortcut {
  command: string
  description: string
  keys: string[]
}

const SHORTCUTS: Shortcut[] = [
  {
    command: 'New thread',
    description: 'Start a new conversation.',
    keys: ['⌘', 'N'],
  },
  {
    command: 'Toggle sidebar',
    description: 'Show or hide the sidebar.',
    keys: ['⌘', 'S'],
  },
  {
    command: 'Toggle thread panel',
    description: 'Show or hide the right panel in a thread.',
    keys: ['Shift', '⌘', 'S'],
  },
  {
    command: 'Toggle Plan mode',
    description: 'Turn Plan mode on or off for the current composer.',
    keys: ['Shift', 'Tab'],
  },
]

export function KeyboardShortcutsSettings() {
  return (
    <section className="py-5">
      <div>
        <p className="text-sm font-medium text-ink">Keyboard shortcuts</p>
        <p className="mt-0.5 text-[13px] text-ink-2">Built-in shortcuts for common actions.</p>
      </div>

      <div className="mt-4 overflow-hidden rounded-card bg-surface">
        <div className="grid grid-cols-[minmax(0,1fr)_auto] gap-3 border-b border-border px-3 py-2.5 text-[12px] font-medium text-ink-3">
          <span>Command</span>
          <span>Keybinding</span>
        </div>
        <div className="divide-y divide-border">
          {SHORTCUTS.map((shortcut) => (
            <div
              key={shortcut.command}
              className="grid grid-cols-[minmax(0,1fr)_auto] gap-3 px-3 py-3"
            >
              <div className="min-w-0">
                <p className="text-[13px] font-medium text-ink">{shortcut.command}</p>
                <p className="mt-0.5 text-[12px] text-ink-3">{shortcut.description}</p>
              </div>
              <div className="flex items-center gap-1.5 self-center justify-self-end">
                {shortcut.keys.map((key) => (
                  <kbd
                    key={`${shortcut.command}-${key}`}
                    className="min-w-6 rounded-full bg-ink/10 px-2 py-1 text-center font-sans text-[12px] font-medium leading-none text-ink-2"
                  >
                    {key}
                  </kbd>
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}
