# Bot Chat Composer

- [x] Make bot and group chat composers look like a messaging app: input and controls share one row and one height.
- [x] Keep the coding composer unchanged.
- [x] Verify rendered geometry in light and dark, run full checks and strict code review.
- [x] Commit the change.

Bot chats are the only composers without the options menu, so `showOptions={false}` became `chat`, and ThreadView derives it from its `chat` renderer instead of taking a second prop. A chat composer is a 52px pill: the input, dictation button and send button sit in one 40px row, and the send button is concentric with the 26px end cap. Text grows upward and the controls stay on its last line. Attachments, contexts and Plan/Goal chips stack above the row. Dictation replaces the whole row, while the textarea keeps focus for Enter and Escape.

Verification: a hidden, non-activating Electron capture of the production `ComposerCard` showed the one-line text centre within 0.4px of the send button centre and the four-line last-line centre within 0.6px. It covered empty, voice, one-line, four-line, Plan, dictation and coding states in light and dark. The capture found the textarea painting over the dictation overlay; that was fixed and re-rendered. 311 frontend tests, typecheck and lint pass.
