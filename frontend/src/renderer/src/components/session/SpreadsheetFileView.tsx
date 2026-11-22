import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import SpreadsheetGrid, { spreadsheetDigitWidth } from '@/components/session/SpreadsheetGrid'
import { sessionFileBytes } from '@/lib/api/sessions'
import { keys } from '@/lib/query/keys'
import { readSpreadsheet } from '@/lib/spreadsheet'

export default function SpreadsheetFileView({ sessionId, path, version, visible, initialSheet, initialSelection }: {
  sessionId: string
  path: string
  version?: string
  visible: boolean
  initialSheet?: string
  initialSelection?: string
}) {
  const [selected, setSelected] = useState(initialSheet ?? '')
  const file = useQuery({
    // A new version of the file is read in place: the workbook on screen stays until the new one is ready.
    queryKey: [...keys.sessionFile(sessionId, path), 'spreadsheet', version],
    queryFn: async ({ signal }) => readSpreadsheet(await sessionFileBytes(sessionId, path, signal), spreadsheetDigitWidth),
    enabled: visible,
    staleTime: 15_000,
    placeholderData: keepPreviousData,
  })
  if (file.isPending) return <p className="p-3 text-[12px] text-ink-3">Loading spreadsheet…</p>
  if (file.isError) return <p className="p-3 text-[12px] text-danger">Couldn&apos;t open the file: {file.error.message}</p>
  const sheet = file.data.find((sheet) => sheet.name === selected) ?? file.data[0]
  if (!sheet) return <p className="p-3 text-[12px] text-ink-3">Empty workbook.</p>
  return <div className="flex h-full min-h-0 flex-col">
    <SpreadsheetGrid key={sheet.name} sheet={sheet} initialSelection={!initialSheet || sheet.name === initialSheet ? initialSelection : undefined} />
    <div className="flex h-9 shrink-0 items-stretch border-t border-border bg-surface text-[12px]">
      <div role="tablist" aria-label="Worksheets" className="scrollbar-quiet flex min-w-0 flex-1 overflow-x-auto">
        {file.data.map((item) => <button key={item.name} type="button" role="tab" aria-selected={item === sheet} onClick={() => setSelected(item.name)} className={`shrink-0 border-r border-border px-4 outline-none focus-visible:bg-primary-soft ${item === sheet ? 'bg-bg font-medium text-primary-strong shadow-[inset_0_-2px_var(--color-primary)]' : 'text-ink-2 hover:bg-list-hover'}`}>{item.name}</button>)}
      </div>
      <span className="flex shrink-0 items-center px-3 text-[11px] text-ink-3">Read only</span>
    </div>
  </div>
}
