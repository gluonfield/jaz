Changes since v0.0.281:

- PDFs in the side panel draw as they should: standard fonts that are not embedded, JPEG 2000 and JBIG2 images, colour profiles and annotations. Cmd+F searches the document with a match count, and selecting text no longer paints over the page.
- Links in documents and chat messages are underlined, so a link is visible without hovering over it.
- Spreadsheets and PDFs open however long the thread has been open, instead of failing with "device token is missing or invalid" after a minute.
- An agent message no longer splits into two blocks when a session or background-task update arrives in the middle of it.
- A child agent's actions are folded into the parent's tool activity.
