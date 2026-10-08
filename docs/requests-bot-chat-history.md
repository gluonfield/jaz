# Bot chat history

- [x] Find why the bot page is laggy.
- [x] Load and render the latest messages, and load more on scroll.
- [x] Verify with tests, a hidden desktop smoke and a measured before/after.

Opening a bot or group fetched its entire history: the loader keeps fetching earlier pages until one holds a user message row, and bot and group conversations keep the person's messages as `room_message` events, so that never happened. A user room message or a turn-opening bot activity now ends the batch like a user message row does.

The chat log rendered every entry and re-rendered every bubble's markdown on each streamed event. It now shares the transcript's history window: the newest 60 entries, 60 more as the reader scrolls up, then earlier pages from the server. Bubbles are memoized. `useSessionHistory` returns one `paging` contract used by the transcript, bot chats and groups.

Measured with 600 chat entries among 3,000 events: one streamed message took 396 ms to render before and 5 ms after.
