import { LOOP_TEMPLATES, type LoopTemplate } from './loopTemplates'

// A minimal, icon-free list of example loops shown inline under the prompt.
// Picking one fills the prompt step.
export function LoopExamples({ onPick }: { onPick: (template: LoopTemplate) => void }) {
  return (
    <div className="space-y-0.5">
      {LOOP_TEMPLATES.map((template) => (
        <button
          key={template.id}
          type="button"
          onClick={() => onPick(template)}
          className="block w-full rounded-control px-2.5 py-1.5 text-left transition-colors duration-150 hover:bg-surface-2"
        >
          <span className="block text-[13px] text-ink">{template.title}</span>
          <span className="mt-0.5 block truncate text-[12px] text-ink-3">{template.description}</span>
        </button>
      ))}
    </div>
  )
}
