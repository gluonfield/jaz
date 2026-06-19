import { LOOP_TEMPLATES, type LoopTemplate } from './loopTemplates'

// A compact, menu-style list of example loops — titles only, so it reads as a
// picker rather than a wall of prompts. Picking one fills the prompt step.
export function LoopExamples({ onPick }: { onPick: (template: LoopTemplate) => void }) {
  return (
    <div className="space-y-0.5">
      {LOOP_TEMPLATES.map((template) => (
        <button
          key={template.id}
          type="button"
          onClick={() => onPick(template)}
          className="block w-full truncate rounded-control px-2.5 py-1.5 text-left text-[13px] text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
        >
          {template.title}
        </button>
      ))}
    </div>
  )
}
