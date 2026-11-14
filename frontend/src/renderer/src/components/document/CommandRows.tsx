import { Fragment } from 'react'
import { MenuRow } from '@/components/ui/Popover'
import type { DocumentCommand } from './documentCommands'

// Commands as menu rows, a rule between groups. selected names the one in
// force and active is the one the keyboard is on.
export function CommandRows({ groups, selected, active, onPick }: { groups: DocumentCommand[][]; selected?: string; active?: DocumentCommand; onPick: (command: DocumentCommand) => void }) {
  return groups.map((group, at) => (
    <Fragment key={group[0].label}>
      {at ? <div className="mx-2 my-1 h-px bg-border" /> : null}
      {group.map((command) => (
        <MenuRow key={command.label} role="menuitem" selected={command.label === selected} active={command === active} onClick={() => onPick(command)}>
          <span className="flex items-center gap-2">
            <command.icon size={14} />
            {command.label}
          </span>
        </MenuRow>
      ))}
    </Fragment>
  ))
}
