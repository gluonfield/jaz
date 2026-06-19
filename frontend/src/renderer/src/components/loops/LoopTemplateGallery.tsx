import { type LucideIcon, PenLine } from 'lucide-react'
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
      {LOOP_TEMPLATES.map((template) => (
        <GalleryRow
          key={template.id}
          icon={template.icon}
          title={template.title}
          description={template.description}
          variant="template"
          onClick={() => onPick(template)}
        />
      ))}
      <GalleryRow
        icon={PenLine}
        title="Start from scratch"
        description="Write your own prompt and schedule."
        variant="blank"
        onClick={onBlank}
      />
    </div>
  )
}

const ROW_BASE =
  'group flex w-full items-center gap-3.5 rounded-card px-3.5 py-3 text-left transition-colors duration-150'

function GalleryRow({
  icon: Icon,
  title,
  description,
  variant,
  onClick,
}: {
  icon: LucideIcon
  title: string
  description: string
  variant: 'template' | 'blank'
  onClick: () => void
}) {
  const blank = variant === 'blank'
  return (
    <button
      type="button"
      onClick={onClick}
      className={`${ROW_BASE} ${
        blank
          ? 'border border-dashed border-border hover:border-primary/50 hover:bg-surface'
          : 'bg-surface hover:bg-surface-2'
      }`}
    >
      <span
        className={`grid size-9 shrink-0 place-items-center rounded-control bg-surface-2 ${
          blank ? 'text-ink-3' : 'text-ink-2 transition-colors group-hover:bg-bg'
        }`}
      >
        <Icon size={17} />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[13.5px] font-medium text-ink">{title}</span>
        <span className="mt-0.5 block truncate text-[12.5px] text-ink-3">{description}</span>
      </span>
    </button>
  )
}
