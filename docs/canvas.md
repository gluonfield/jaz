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
| `points` | `stroke` | the drawn line as `[x, y]` pairs measured from the item's `x` and `y`; resizing stretches it |
| `weight`, `highlight` | `stroke` | line width in pixels (4 by default); `true` draws it wide and translucent |
| `color` | `stroke`, edges | an ink name or a CSS colour |
| `route` | edges | `curve` (default), `elbow`, `straight` |
| `dash` | edges | `solid` (default), `dashed`, `dotted` |
| `fromEnd`, `toEnd` | edges | `none`, `arrow`, `circle`; an edge starts plain and ends in an arrow by default |
| `locked` | any node | `true` pins the item: it cannot be moved, resized, edited or deleted until unlocked |
| `z` | any node | stacking order; higher is in front, negative is behind |
| `data` | component and HTML `file` | any JSON value the card saves for itself |
| `place` | any node | `{"below": "<id>"}` or `right`, `above`, `left`, with optional `gap` (40) and `align` (`start`, `center`, `end`) |

```json
{"id":"gate","type":"text","text":"Ship?","shape":"diamond","color":"yellow","x":0,"y":0,"width":160,"height":120}
{"id":"owners","type":"table","rows":[["Step","Owner"],["Draft","Ana"]],"x":200,"y":0,"width":320}
```

Selecting an item shows its controls above it: shape, size, bold, italic, strikethrough, code, bulleted and numbered lists, alignment and colour for text; size and rows and columns for a table; route, line, ends and colour for a connector; ink and weight for a drawn line. Every item also has Lock and, under More, Duplicate, Bring to front (`Mod+]`), Send to back (`Mod+[`) and Delete. Selecting several items offers the six alignments and the same actions for all of them.

Double-clicking text opens it for typing as formatted words when it uses only paragraphs, lists, bold, italic, strikethrough, code and links; anything else, such as a heading or a quote, opens as Markdown source. Double-click a table cell to edit it; Enter keeps it, Tab keeps it and moves on, Escape leaves it unchanged. The dock's Shape (`R`) and Table (`B`) tools add new ones. The Pen (`P`) draws lines until another tool is chosen, with inks, three weights and a highlighter above the dock; the Eraser (`E`) removes each line it is dragged across.

A component or HTML card keeps its state in its node's `data`. Inside the sandbox `window.data` holds the saved value when the card loads, `saveData(value)` saves a new one (up to 200,000 characters of JSON) as an edit the user can undo, and `onData(listener)` is called when the value changes outside the card, by undo or by an agent editing the file. The card is not reloaded for a change to its data.

An item with a `place` gets its `x` and `y` from the measured size of the item it names once its page is on screen; the client then saves them and removes `place`. Whenever a page has drawn and gone quiet, the client also sends what it measured with `PUT /v1/sessions/{session}/canvas/layout` (`page`, `items` as `{id, x, y, width, height}`, and `issues`), and the server keeps the latest per page in `layout.json` beside `canvas.json`, one item and one issue per line. Issues name overlapping items, items across a section's edge, content shrunk to fit its box and places that could not be worked out. Agents read this file to check a layout they wrote.

Agents may also edit the file directly. Re-read it before editing, change only the intended pages/items, and retain IDs. Missing names in agent-written pages default to their `Page N` position.

Item links use `/sessions/{thread}?page={page-id}&item={item-id}`, URL-encoding both IDs. A current-thread link may use just `?page=...&item=...`. An item-only link or unknown page/item reports an error. The active page accompanies messages even without selected items; this location alone does not permit an empty message. Canvas selections carry both page and node IDs into the message and agent prompt; historical selections without a page refer to `page-1`.
