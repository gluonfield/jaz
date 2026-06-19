import { LOOP_TEMPLATES, type LoopTemplate } from './loopTemplates'

// The examples view: a list of ready-made loops. Picking one fills the create
// form and returns to it — there is no blank row because the form is already
// the default starting point.
export function LoopTemplateGallery({ onPick }: { onPick: (template: LoopTemplate) => void }) {
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
    </div>
  )
}
