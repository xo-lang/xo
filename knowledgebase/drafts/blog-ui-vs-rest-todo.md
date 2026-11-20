# Draft: One small site built two ways in Xo

Status: Draft (xo-4c, 2026-11-19, task #432, for the owner; not
published). Figures come from `examples/ui-todo/measure`,
`examples/todo-rest/measure` and a size script run on the Mac Studio
(darwin/arm64) at the commit that adds this draft; the run outputs are
in the task record. In process timings on one machine: read them as
ratios, not as a capacity plan.

---

Xo gives you two ways to put a web app in front of people. The
traditional way: a back end that speaks JSON, and a front end of HTML
and JavaScript, which Xo can embed in the server binary (`embed`,
decision 0304) so the whole site is still one file to deploy. Or
`std/ui`: you write the model, the messages, `update` and `view` once in
Xo, the server runs them, and the same code built for wasm runs in the
browser to show each change before the server confirms it.

To compare them we built one small app both ways: a shared todo list
plus three seats anyone can reserve (the seats are the part where two
people can race). `examples/ui-todo` is the std/ui version;
`examples/todo-rest` is the traditional one, a std/http JSON API with an
embedded page and script that polls for changes every 2 seconds, the
same interval the std/ui client uses.

## How much code

| | std/ui | JSON API + page |
|---|---|---|
| Lines (no comments or blanks) | 112 | 217 |
| Languages | Xo | Xo, HTML and CSS, JavaScript |
| Files | 3 | 4 |

The std/ui count includes its CSS (in `todo.xo`); the traditional count
includes the same CSS in `index.html`. The gap is mostly the
JavaScript: 90 lines that render the state, send each action, show
errors, and poll. Those 90 lines do less than std/ui does. The page
waits for the server on every click instead of predicting; it drops an
action made while offline instead of queuing it; it has no version
push. Adding those would mean a client side model of the state and a
second copy of every rule, in JavaScript, kept in step with the Xo one
by hand. That second copy is what std/ui removes: the seat rule (no
seats left, no name twice) is written once and runs in both places.

## What the browser downloads

First load, bytes on the wire as served:

| | std/ui, wasm client | std/ui, thin client | JSON API + page |
|---|---|---|---|
| Page | 1,128 | 1,051 | 1,566 |
| Runtime (`ui.js`) | 8,641 | 8,641 | |
| Client (`app.wasm` or `app.js`) | 93,060 | | 2,746 |
| First state | (in the page) | (in the page) | 55 |
| Total | 102,829 | 9,692 | 4,367 |

std/ui's server compresses its responses; the traditional server here
does not (its files go through `http.serve_content`), and gzip would
bring its total to about 2,000 bytes. The wasm client is the cost of
prediction: about 93 KB, served once with a content stamp and cached
for good, so a returning visitor downloads it again only after a
rebuild. The thin client (`xo run ./server -- :8080 thin`) shows the
server's rendered view and skips the wasm.

## What the server does per action and per viewer

The measure programs drive each server's routes in process, with no
network: one viewer adds todos one at a time, then 1,000 idle viewers
each check for changes once. Three runs each; the ranges are across
runs.

| | std/ui, thin client | std/ui, wasm client | JSON API |
|---|---|---|---|
| One add, 200 todos in the list | 109 to 122 us | 13 to 22 us | 3 us |
| Reply to an add, 10 todos | 2,167 B | 408 B | 269 B |
| Reply to an add, 200 todos | 28,476 B | 4,350 B | 4,209 B |
| One idle check | 1.2 to 1.9 us | 1.3 to 2.1 us | 0.4 us |
| Reply to an idle check | 151 B | 151 B | 10 B |

For a thin client std/ui's server renders the view and sends it, so
the reply grows with the page: the price of a client with no
JavaScript of the app's own. A wasm client gets the model and renders
it itself, which brings its replies to the JSON API's size. Writing
this comparison found that the wasm client was being sent both, the
model and the server's render (32,673 B at 200 todos), and std/ui now
leaves the render out for it (task #451). The rest of the gap to the
JSON API is std/ui's sync bookkeeping: acks, the version, the digest
that lets an idle check answer "same".

The idle cost is what decides how many open tabs a server holds. Both
are small: 1,000 viewers checking every 2 seconds cost std/ui about 1
ms of server CPU per second, and the JSON API about 0.2 ms. std/ui
answers an idle check with "same" (all 1,000 checks did) and never
renders for it.

## Hosting

The traditional front end is static: `index.html` and `app.js` can go
on any static host or CDN, with only `/api` on a server, or stay
embedded in the binary as here. std/ui renders the page on the server
for each visit, so the page needs the Xo server; its runtime and wasm
client are static, content stamped files that a CDN can cache.

## Sign in

std/ui takes one function, `ui.host(..., auth: fn(http.Request) ->
Str?)` (decision 0664), and every message the client sends carries the
user that function returned; `allow` and `redact` decide what each user
may send and see, in the same Xo as the rest of the app. The
traditional server checks the session in each handler (or in one
middleware), and the JavaScript has to handle a 401 on every call.
Neither is hard; std/ui's is in one place.

## When to pick which

- **std/ui** when the app is mostly shared state with rules (who may
  take the last seat), when you want clicks to show at once and
  offline actions to queue, and when one language for the whole app
  matters more than 93 KB on the first visit.
- **JSON API plus a page** when the front end is someone else's (a
  mobile app, a partner, a team that writes React), when the page must
  be static hosted, or when the clients cannot run wasm and the
  replies must stay small as the page grows: a thin std/ui client is
  sent a render, the API sends data.

Both live in one Xo binary, and a site can use both: std/ui mounts
under a prefix (`mount(r, "/todo")`) beside ordinary routes.

## Open questions from the numbers

- The figures are in process on one machine. A run over the network
  with many concurrent viewers would show the websocket push's cost,
  which this comparison does not measure.
