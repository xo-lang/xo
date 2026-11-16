# 0668: std/ui moves between pages without a full load

Status: Accepted (owner, 2026-11-15; option 1 first, option 2 only when
an app needs state across pages)

## Context

Each std/ui page was a full document load: following a link between an
app's pages started the runtime, its socket, its timers and the wasm
client module again, and showed a blank page meanwhile. xw's dashboard
has 7 std/ui hosts, one per page, plus plain pages (search, sign in).
Other frameworks follow links in place: Turbo Drive, htmx `hx-boost`,
LiveView `navigate`, and the client routers of SPA frameworks.

## Decision

The runtime follows a link itself when all of these hold: a plain left
click with no modifier key, an `<a href>` on the same origin, no
`target` other than `_self`, no `download`, no `data-xo-reload`, and not
a link to a fragment of the current page. It then:

1. fetches the URL (same-origin credentials); the reply must be HTML
   from the same origin (after redirects) holding `#xo-ui` and
   `#xo-state`, or the runtime does an ordinary navigation to it, so a
   plain page, a sign-in redirect or a failed fetch is a full load;
2. pushes a history entry (the URL after redirects) and records the old
   page's scroll position in its entry;
3. stops the current page's runtime: its listeners, socket and view go;
4. patches the new page's head and body in (the morph of 5c377ace8), so
   the title, CSS and preload follow and the already loaded runtime
   script is not run again;
5. starts the runtime for the new page (its own sync and live URLs,
   client id, queue, socket), at the top of the page.

Back and forward do the same without the push, restoring the entry's
scroll position. A newer navigation cancels an older one still loading.

Each page's queue stays its own (its localStorage key is the path and
sync URL, as before): a page left with Msgs not yet acknowledged keeps
syncing them to its own endpoint in the background, without a view,
until they drain, and starting that page again takes the queue over. A
compiled client module is kept per URL, so returning to a wasm page
does not fetch or compile it again.

Option 2 (one App routing by URL in its model, like Elm's
`Browser.application`) is not built: it needs every page in one model,
authorization and redaction per route, and a restructured app. It waits
for an app that needs state across pages.

## Consequences

- Measured in headless Chrome (2026-11-15, a server with two counter
  pages, the todo page with its wasm client and a plain page; driver
  ~/xo-lead/notes/stdui/e2e/drive-nav.mjs): a link switches pages in
  4 to 11 ms with the window kept; back and forward keep each page's
  state; the new page starts at the top and back restores 2,000 px of
  scroll; a click on the wasm page works right after navigating; two
  Msgs queued on page A with its sync blocked are not sent to page B
  and reach A 1.2 s after the block lifts, while B is shown; a plain
  page and a `data-xo-reload` link are full loads (17/17). The todo
  checks (21/21) and morph checks (3/3) still pass.
- Apps need no change. A link that must reload (a page outside std/ui
  served from the same origin as HTML with those ids, or a download
  route answering HTML) takes `data-xo-reload`.
- Scripts in a fetched page do not run (the patch inserts parsed nodes);
  std/ui pages carry only the runtime and the state, which is data.

## Amendment: stamped assets and new builds (2026-11-15)

With navigation an open tab no longer reloads, so it kept the runtime
and wasm client it started with after a redeploy. Now:

- The runtime and the client are served at URLs stamped with a
  SHA-256 prefix of their content (`/_xo/ui.js?v=…`,
  `/_xo/app.wasm?v=…`), `Cache-Control: public, max-age=31536000,
  immutable` when the request names the current stamp and `no-cache`
  otherwise, with an ETag; the runtime is gzipped too. A rebuild
  changes the URL.
- The page's root carries `data-xo-build` (the runtime stamp, plus the
  client stamp) and every sync reply carries `build`. A page that sees
  another build sets `data-xo-stale` on the root element and reloads
  once its queue is sent and no input is being typed in (checked on
  each sync and every 2 s).
- A link to a page with another runtime stamp is a full load. A page
  with another client stamp but the same runtime is swapped in place:
  its runtime loads the new client from the new URL.
- The page syncs when its live socket opens: after a server restart the
  socket reconnected and the page waited for its 30 s check, so changes
  made meanwhile and a new build showed up late
  (TestSocketOpenSyncs).

Measured in headless Chrome (~/xo-lead/notes/stdui/e2e/drive-stamp.mjs,
two builds of a server whose shared app differs, restarted under two
open tabs; 6/6): an idle page reloads onto the new build 1.1 to 1.3 s
after the restart; a page with half typed text keeps it and is marked
stale; it reloads 0.1 s after the input is emptied. The navigation
(17/17), todo (21/21) and morph (3/3) checks pass.

## Alternatives

- Option 2 now: see above.
- Fetch only the view (a JSON route) instead of the page: saves the
  head and state bytes (a few hundred) but needs a new route per host
  and a second code path for the head; the page GET is already gzip
  and ETag cached.
- Leave the old page's queue in localStorage only (sent when the page
  is next opened): loses nothing but delays the Msgs indefinitely; the
  background drain sends them at once.
