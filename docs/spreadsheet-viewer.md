# Read-only spreadsheet viewer

Use the existing SheetJS parser with a virtualized DOM grid. The only added runtime dependency is [saxes 6.0.0 (ISC)](https://github.com/lddubeau/saxes), which reads XLSX style IDs and freeze metadata from the already-loaded ZIP. Its upstream repository was archived in December 2025; the pinned, strict parser is confined to this replaceable metadata boundary. Legacy XLS frozen panes come from the existing CFB/BIFF stream; the viewer never writes the workbook or sends it to a conversion service.

## Library decision

Bundle figures below are **estimates from measured minified browser entry bundles**, including grid CSS and transitive code, excluding shared React, React DOM, SheetJS and React Query. They are not package unpacked sizes. Versions checked 16 November 2026; application chunking can change the totals.

| Candidate | Commercial licence and maintenance | Added bundle, minified / gzip | Fit |
| --- | --- | --- | --- |
| Custom DOM over SheetJS | Existing [SheetJS 0.20.3 is Apache-2.0](https://docs.sheetjs.com/docs/miscellany/license/); saxes is ISC. Jaz owns the small viewer and pinned parser boundary. | **≈46 / 14 kB above the old viewer** | Selected: direct merges, variable geometry, native text, theme tokens and read-only selection. Canvas would need separate text measurement, accessibility and hit-testing code. |
| Glide Data Grid 6.0.3 | [MIT, commercial use permitted](https://github.com/glideapps/glide-data-grid); [latest stable published February 2024](https://registry.npmjs.org/@glideapps/glide-data-grid). Stable peer metadata ends at React 18, although main documents React 19. | ≈347 / 115 kB | Strong canvas virtualization, selection and column spans; worksheet conversion, vertical merges, file formatting and tabs still need adaptation. |
| FortuneSheet 1.0.4 | [MIT](https://github.com/ruilisi/fortune-sheet); [latest published November 2025](https://registry.npmjs.org/@fortune-sheet/react). Its README acknowledges some outdated documentation. | ≈2,867 / 612 kB | Broad spreadsheet features, merges, formatting and selection, but brings an editor/formula system and a second workbook model for this viewing task. |
| Jspreadsheet CE 5.0.4 | [CE is MIT](https://github.com/jspreadsheet/ce); [latest published August 2025](https://registry.npmjs.org/jspreadsheet-ce). Pro is a separate product. | ≈508 / 137 kB | Familiar DOM spreadsheet controls and styling; workbook conversion and React lifecycle integration remain. Adds editing/formula dependencies that this viewer does not use. |
| Univer sheets-core 1.5.1 | [Open-source core is Apache-2.0](https://github.com/dream-num/univer); [current release published November 2026](https://registry.npmjs.org/@univerjs/preset-sheets-core). | ≈10,913 / 2,596 kB | Comprehensive canvas office engine with good spreadsheet coverage. Its [Office import/export requires Pro exchange plugins and a conversion backend](https://docs.univer.ai/guides/sheets/features/import-export); excessive scope for a local read-only preview. |

## Behaviour and limits

- Sticky column letters and row numbers, file dimensions, hidden rows/columns, merges, frozen rows/columns, viewport-filling blank cells, bottom sheet tabs, click/drag/Shift selection, keyboard movement and escaped TSV copy. Only visible cells are mounted; the 1,000-row cap is removed.
- `SpreadsheetFileView` accepts optional `initialSheet` and `initialSelection` (A1 cell or range). It adds no link syntax. The value bar exposes the complete raw value or formula; the grid shows formatted cached results.
- XLSX supports available bold, italic, font name/size/colour, solid fill, horizontal/vertical alignment and explicit wrapping. Unwrapped text keeps one line and overflows into empty neighbours; occupied or merged neighbours stop it. Neutral colours follow Jaz light/dark tokens. Fonts must exist on the client; column character widths use a conventional 7px digit metric, so font-dependent pixel differences remain.
- XLS supports SheetJS-exposed values, number/date text, merges, dimensions and solid fills, plus BIFF frozen panes. Its font/alignment/wrap metadata is not fully exposed by SheetJS CE. CSV preserves text/leading zeroes and has no workbook formatting. [SheetJS documents these extraction limits](https://docs.sheetjs.com/docs/api/parse-options/).
- Cached formulas are displayed, never recalculated. Conditional formatting, rich-text runs, custom borders, images/charts, rotation and independent split-window views are outside this viewer. XLS frozen panes use the [BIFF Window2 flag](https://learn.microsoft.com/en-us/openspecs/office_file_formats/ms-xls/80576ec8-e7f0-4c99-90a7-d850587bc97b) and [Pane coordinates](https://learn.microsoft.com/en-us/openspecs/office_file_formats/ms-xls/9dcaa80b-6315-4aea-a802-872ccd4e8d1b), rather than treating ordinary splits as freezes.

## Verification

- Both supplied tender workbooks parsed without source-byte changes: PDG has three sheets (21/19/25 merges); Adani has one sheet with seven frozen rows and three frozen columns. Real light/dark renders and Adani's Wingdings ticks were inspected in the integrated browser.
- Browser checks covered merged keyboard selection, drag selection/native TSV copy, complete value/formula display, initial sheet/range selection, and stable frozen headers after scrolling both axes. A 50,000-row CSV selected row 40,000 with about 510–525 mounted cells; sampled React renders took about 3ms in a 960px panel.
- Focused spreadsheet tests cover XLSX/XLS/CSV, styles/source immutability, dimensions, hidden rows, merges, freeze-versus-split metadata, selection/copy, overflow and virtualization. Typecheck, focused lint and web build pass. The initial broad test run passed 503 tests with four Electron import failures; installing the existing local Electron runtime unblocked all four files, whose nine tests then passed through hidden harnesses.
- Thermo-nuclear self-review applied: separate parsing/BIFF/geometry/rendering responsibilities, memoized visible cells, no empty text subtrees, unique cell IDs across frozen panes, and merged-cell keyboard movement.

Screenshots and detailed check receipts are kept under the gitignored `runs/spreadsheet-grid-viewer/` directory. The real workbooks are not copied into this repository.
