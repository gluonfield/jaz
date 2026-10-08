# Picker Fast Mode, light colours and folder drops

- [x] Show that Fast Mode is on: a labelled ⚡ Fast switch in the picker and a ⚡ on the closed picker.
- [x] Remove the oversized Fast Mode button that added a 40px header row to the picker.
- [x] Review light-mode colours: the near-black thumb and grey stop dots on the blue fill.
- [x] Let folders be dropped onto the composer.
- [x] Verify in the desktop smoke suite, run full checks and review.

The Fast switch now sits at the right of the 24px model row, so the picker is the same height with or without it. Keyboard order follows the layout: model, Fast, slider. The slider thumb is white in every theme. Stop dots on the filled track use the on-primary colour, which follows custom accents.

A dropped folder becomes an `@folder` mention of its absolute path when the desktop app talks to the local backend. Agents then work on the real folder. Other clients and remote backends cannot see client paths, so the folder appears as a failed attachment explaining that folders need the desktop app with a local backend.

Verified: typecheck, lint, 310 frontend tests, web build and the desktop `--model-picker` smoke suite. The smoke drops a real directory through Chromium's drag pipeline and checks the sent `[@Folder fixture](<path>)` and the remote refusal. It also checks the closed picker announces Fast Mode, the Fast switch does not enlarge the header, and the Tab order. Screenshots were checked in stock light, a light custom accent and dark.

Known limit: two different folders with the same name share one mention token, because composer tokens are keyed by their display text.
