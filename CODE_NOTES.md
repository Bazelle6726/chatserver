# Code notes

Comments that left the code, kept word for word. Each entry says where the
comment was (file, the enclosing function or type, and the line number when
it moved; lines drift, so go by the name) and why it left.

## internal/hub/pool.go

### `type Pool struct`, field `banned` (line 18 at move)
Why moved: stale: the moderation admin moved into the service layer, so there
is no Admin type any more. The comment was reworded in place in commit
df0602c (now "e.g. a moderation.List's IsBanned method"); this is the
original text.
> e.g. an Admin's IsBanned method

## internal/service/service.go

### `func withSuffix` (line 539 at move)
Why moved: stale: the suffix is now 4 decimal digits, not hex (the fuzz fix
for a Cyrillic player name in df0602c). The comment was reworded
in place in commit df0602c (now "4 random digits"); this is the original
text.
> withSuffix adds "-" and 4 random hex digits to base, shortening base if
> needed so the result is still a valid length.

## web/src/App.jsx

### `function App` (lines 15-17 at move)
Why moved: stale: useCallback is banned by the ESLint `no-restricted-imports`
rule, and no child uses these functions in effect dependencies any more (the
live feed lives in roomFeed.js and keeps the callbacks it was created with).
> useCallback keeps these the same function between renders. Children use
> them in effect dependencies, and a new function each render would restart
> those effects (reconnecting the live feed) every time the status changed.

## web/src/components/ChatRoom.jsx

### `function ChatRoom`, inside its first `useEffect` (line 49 at move)
Why moved: stale: the effect is gone. The feed now stops in roomFeed.js when
the last component unsubscribes from useSyncExternalStore.
> Cleanup: runs when the room changes or the component goes away.

## web/src/useUserNames.js

### `function useUserNames` (lines 4-5 at move)
Why moved: stale: the file was removed. Messages now carry `author_name`, so
the web app no longer looks names up one by one.
> Messages only carry the author's ID, so look names up once and remember
> them. Returns nameOf(id): the name if known, else the ID (and fetches it).

## web/src/roomFeed.js

### Top of the module (lines 1-4 at move)
Status: explains
> A room's live feed, kept outside React: this module owns the network work
> (joining, the EventSource, the history), and components read the result
> with useSyncExternalStore. The feed starts when the first component
> subscribes and stops when the last one leaves.

### `const feeds` (line 7 at move)
Status: explains
> room ID -> feed

### `function getRoomFeed` (lines 11-14 at move)
Status: explains
> getRoomFeed returns the feed for room, creating it the first time. on holds
> the app's callbacks: joined(), status(text), error(err). The ones given
> when the feed is created are kept; they only call React state setters,
> which never change.

### `function createFeed`, `const add` (line 35 at move)
Status: explains
> Add messages, skipping any we already have (by ID), oldest first.

### `function createFeed`, `async function start` (line 49 at move)
Status: explains
> 409 = already a member

### `function createFeed`, `async function start` (line 54 at move)
Status: explains
> Watch BEFORE loading the history, so nothing posted in between is lost.

### `function createFeed`, `function stop` (lines 69-70 at move)
Status: explains. Until 2026-11-26 this promise did not hold: a start()
still waiting on the history request kept going after stop() and put the
old messages back. stop() now bumps `run`, and start() checks it after
every wait.
> Stopping also forgets the messages, so whoever signs in next on this
> browser never sees the last person's rooms.
