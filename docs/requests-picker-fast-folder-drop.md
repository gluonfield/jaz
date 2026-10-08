# Picker Fast Mode, light colours and folder drops

- [x] Show that Fast Mode is on: a labelled ⚡ Fast switch in the picker and a ⚡ on the closed picker.
- [x] Remove the oversized Fast Mode button that added a 40px header row to the picker.
- [x] Review light-mode colours: the near-black thumb and grey stop dots on the blue fill.
- [x] Let folders be dropped onto the composer.
- [x] Verify in the desktop smoke suite, run full checks and review.
- [x] Make the voice/send button icon white on the Codex accent and fix colours overall.
- [x] Stop desktop test runs from appearing on screen or taking focus.

The Fast switch now sits at the right of the 24px model row, so the picker is the same height with or without it. Keyboard order follows the layout: model, Fast, slider. The slider thumb is white in every theme. Stop dots on the filled track use the on-primary colour, which follows custom accents.

A dropped folder becomes an `@folder` mention of its absolute path when the desktop app talks to the local backend. Agents then work on the real folder. Other clients and remote backends cannot see client paths, so a toast explains that folders need the desktop app with a local backend.

Verified: typecheck, lint, 310 frontend tests, web build and the desktop `--model-picker` smoke suite. The smoke drops a real directory through Chromium's drag pipeline and checks the sent `[@Folder fixture](<path>)` and the remote refusal. It also checks the closed picker announces Fast Mode, the Fast switch does not enlarge the header, and the Tab order. Screenshots were checked in stock light, a light custom accent and dark.

Known limit: two different folders with the same name share one mention token, because composer tokens are keyed by their display text.

## Accent colours

The Codex preset accent `#339cff` has luminance 0.317, just above the 0.3 cutoff that chose dark text, so every accent fill (send, voice, primary buttons, checked switches and checkboxes, the Fast pill) drew near-black content. The cutoff is now 0.34: Codex blue, Raycast red and Everforest olive get white text; every other preset keeps its choice, including Jaz dark. Switch knobs are white in both states. The composer menu's Plan, Goal and Fast Mode rows now share one `SwitchRow`, replacing two hand-copied rows and a taller Fast Mode row, and the import dialog's hard-coded `#3686ff` override is gone. A component gallery was reviewed in stock and Codex, light and dark. The desktop smoke applies the real Codex preset and checks white text, and checks that a light amber accent keeps dark text.

## Quiet desktop tests

The smoke window was visible and every simulated pointer event called `window.focus()`, so it kept jumping to the front. It now runs as a non-activating app with a hidden main window, popups are opened hidden, and focus is emulated over CDP. During a passing run, an on-screen window sampler saw no harness windows and the frontmost app never changed.

The full desktop suite stops intermittently in background-browser lifecycle stages on this machine under heavy load (load average above 20). Unmodified `main` failed the same way under the same load.
