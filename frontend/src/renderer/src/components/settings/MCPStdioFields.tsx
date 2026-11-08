import { Input } from '@/components/ui/Input'
import type { Draft } from '@/components/settings/MCPServerDraft'

export function MCPStdioFields({ draft, onChange }: { draft: Draft; onChange: (draft: Draft) => void }) {
  const textareaClass = 'w-full rounded-control border border-border bg-surface px-3 py-2 font-mono text-[12px] text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent'
  return (
    <div className="space-y-4">
      <label className="block">
        <span className="mb-1 block text-[12px] font-medium text-ink-2">Executable</span>
        <Input
          placeholder="/path/to/mcp-server"
          value={draft.command ?? ''}
          onChange={(event) => onChange({ ...draft, command: event.target.value })}
        />
        <span className="mt-1 block text-[12px] text-ink-3">Runs on the machine hosting the Jaz server. Use an executable path or a command on its PATH.</span>
      </label>
      <label className="block">
        <span className="mb-1 block text-[12px] font-medium text-ink-2">Arguments (JSON array)</span>
        <textarea
          className={textareaClass}
          rows={2}
          spellCheck={false}
          value={draft.argsJSON}
          onChange={(event) => onChange({ ...draft, argsJSON: event.target.value })}
        />
        <span className="mt-1 block text-[12px] text-ink-3">For example: ["--mcp"]. Each string is one argument; no shell is used.</span>
      </label>
      <label className="block">
        <span className="mb-1 block text-[12px] font-medium text-ink-2">Environment (JSON object)</span>
        <textarea
          className={textareaClass}
          rows={3}
          autoComplete="off"
          spellCheck={false}
          value={draft.envJSON}
          onChange={(event) => onChange({ ...draft, envJSON: event.target.value })}
        />
        <span className="mt-1 block text-[12px] text-ink-3">Optional overrides for the inherited server environment. Values are stored encrypted; null keeps a saved value.</span>
      </label>
    </div>
  )
}
