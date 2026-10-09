# Chat Tables

- [x] Redesign Markdown tables after the reference screenshot: no cell grid, left-aligned bold headers, top-aligned cells, rows divided by rules, on a card.
- [x] Keep text after a wrapped link on the link's last line (", Curzon Building" was starting a new line).
- [x] Verify rendered tables in light and dark, including GFM column alignment and a wide table scrolling in a narrow column; run full checks and strict review.
- [x] Commit both changes.

Tables now wear the code block's card (surface fill, 60% border ring, card radius) and keep scrolling inside it when wide. Cells have no column lines; a 2px rule closes the header and 1px rules divide body rows, inset to the card's padding. Headers are left-aligned and cells top-aligned. Table text uses the prose size instead of 13px. Explicit GFM alignment still wins.

Web links were `inline-flex`, so a wrapped link took the full line width. They now flow inline with their favicon. File links followed in the review follow-up (`docs/requests-review-follow-ups.md`).

Verification: hidden, non-activating Electron captures of the production `RenderedMarkdown` showed the reported events table, the reference project table and a right-aligned price table in light and dark. A 608px table in a 420px column scrolled inside its card and kept 14px end padding. 311 frontend tests, typecheck and lint pass.
