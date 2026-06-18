import { PenLine } from 'lucide-react'
import { LOOP_TEMPLATES, type LoopTemplate } from './loopTemplates'

// The first step of creating a loop: pick a template to prefill the form, or
// start from a blank prompt. Both paths land in the same LoopForm afterwards.
export function LoopTemplateGallery({
  onPick,
  onBlank,
}: {
  onPick: (template: LoopTemplate) => void
  onBlank: () => void
}) {
  return (
    <div className="space-y-2">
      {LOOP_TEMPLATES.map((template) => {
        const Icon = template.icon
        return (
          <button
            key={template.id}
            type="button"
            onClick={() => onPick(template)}
            className="group flex w-full items-center gap-3.5 rounded-card bg-surface px-3.5 py-3 text-left transition-colors duration-150 hover:bg-surface-2"
          >
            <span className="grid size-9 shrink-0 place-items-center rounded-control bg-surface-2 text-ink-2 transition-colors group-hover:bg-bg">
              <Icon size={17} />
            </span>
            <span className="min-w-0 flex-1">
              <span className="block truncate text-[13.5px] font-medium text-ink">
                {template.title}
              </span>
              <span className="mt-0.5 block truncate text-[12.5px] text-ink-3">
                {template.description}
              </span>
            </span>
          </button>
        )
      })}

      <button
        type="button"
        onClick={onBlank}
        className="group flex w-full items-center gap-3.5 rounded-card border border-dashed border-border px-3.5 py-3 text-left transition-colors duration-150 hover:border-primary/50 hover:bg-surface"
      >
        <span className="grid size-9 shrink-0 place-items-center rounded-control bg-surface-2 text-ink-3">
          <PenLine size={17} />
        </span>
        <span className="min-w-0 flex-1">
          <span className="block text-[13.5px] font-medium text-ink">Start from scratch</span>
          <span className="mt-0.5 block text-[12.5px] text-ink-3">
            Write your own prompt and schedule.
          </span>
        </span>
      </button>
    </div>
  )
}
