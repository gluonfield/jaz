# Canvas pages

A canvas thread stores `canvas.json` in its canvas directory. Pages have stable IDs and editable names; each page contains a JSON Canvas 1.0 board. Page IDs are nonempty and have no surrounding whitespace; internal spaces are allowed. Item IDs belong to their page, so different pages may use the same item ID.

```json
{
  "pages": [
    {"id":"page-1","name":"Page 1","nodes":[],"edges":[]},
    {"id":"costs","name":"Costs","nodes":[],"edges":[]}
  ]
}
```

The page selector switches pages; the adjacent plus adds a page. Right-click or press and hold the selector for Rename page and Delete page. Keyboard users can press Shift+F10 on the selector. At least one page must remain. New pages default to `Page N`; renaming keeps the page ID. Page selection, viewport and editing state belong to the client, so an agent editing another page does not switch the user's page.

Existing root-level `nodes` and `edges` load as `page-1`, named `Page 1`. Reading leaves the file unchanged; the next successful patch writes the pages schema while preserving the original items. Invalid documents are reported rather than overwritten.

`GET /v1/sessions/{session}/canvas` returns `version`, `dir` and `pages`. Each page includes its own `files` and compiled `components`, keyed by its item IDs. `PATCH` requires a `page` ID and accepts the existing `nodes`, `edges` and `remove` item changes plus an optional `name`. Creating a page explicitly requires `create:true`; a missing or unknown target otherwise fails. Blank rename requests fail.

Deleting a page uses `{"page":"costs","delete":true}` without other changes. Deleting the final page fails without changing the document. The client selects the first remaining page when its current page is deleted.

```json
{"page":"costs","create":true,"name":"Costs"}
```

Free text, tables and Markdown or CSV files flow at their `width`: the board draws them as tall as their content and treats `height` as the most room they may take, scaling content that needs more. A missing `height` sets no limit: text the user types has none, and resizing one of these items sets its width and removes its `height`. Every other item fills its `width` and `height` exactly.

Jaz reads these fields beyond JSON Canvas 1.0. Unknown values are ignored, so a board stays readable elsewhere.

| Field | On | Values |
| --- | --- | --- |
| `size` | `text`, `table` | `small` (14 px, default), `body` (16), `large` (28), `title` (48), `huge` (96) |
| `textAlign` | `text` | `left`, `center`, `right` |
| `ink` | free `text` | `gray`, `red`, `orange`, `green`, `blue`, `purple`, or a CSS colour |
| `color` | `text` | `white`, `red`, `orange`, `yellow`, `green`, `blue`, `purple`, the JSON Canvas presets `1`–`6`, or a CSS colour; makes a sticky note, or fills a shape |
| `shape` | `text` | `rect`, `rounded`, `pill`, `ellipse`, `diamond`, `hexagon`, `parallelogram`, `document`, `database`, `queue`, `cloud`, `person`; words inside an outline |
| `hollow` | shaped `text` | `true` keeps the outline without a fill |
| `rows` | `table` | array of rows, each an array of cell strings with inline Markdown |
| `header` | `table` | `false` when the first row is not a heading |

```json
{"id":"gate","type":"text","text":"Ship?","shape":"diamond","color":"yellow","x":0,"y":0,"width":160,"height":120}
{"id":"owners","type":"table","rows":[["Step","Owner"],["Draft","Ana"]],"x":200,"y":0,"width":320}
```

Selecting a text item or table shows its controls above it: shape, size, bold, italic, list, alignment and colour for text; size and rows and columns for a table. Double-click a table cell to edit it; Enter keeps it, Tab keeps it and moves on, Escape leaves it unchanged. The dock's Shape (`R`) and Table (`B`) tools add new ones.

Agents may also edit the file directly. Re-read it before editing, change only the intended pages/items, and retain IDs. Missing names in agent-written pages default to their `Page N` position.

Item links use `/sessions/{thread}?page={page-id}&item={item-id}`, URL-encoding both IDs. A current-thread link may use just `?page=...&item=...`. An item-only link or unknown page/item reports an error. The active page accompanies messages even without selected items; this location alone does not permit an empty message. Canvas selections carry both page and node IDs into the message and agent prompt; historical selections without a page refer to `page-1`.
