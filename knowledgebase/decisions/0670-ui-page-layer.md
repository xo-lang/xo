# 0670: std/ui page layer: any page, forms and frames without full loads

Status: Accepted (owner, 2026-11-15)

## Context

std/ui is meant to grow past one dashboard (owner, 2026-11-15: "we are
going to grow in complexity so let's not just build for this UI"). Apps
grow in three layers, and frameworks cover different ones:

| Layer | What it handles | Done well by |
|---|---|---|
| 1. Pages and navigation | links, GET and POST forms, history, regions, new builds | Turbo Drive and Frames, htmx `hx-boost` |
| 2. Shared live data | state many people see and change, offline, instant | std/ui Apps (0659), Replicache, Zero |
| 3. Per-viewer state | search, filters, drafts, "my" view | LiveView, Elm's URL in the model |

Decision 0668 covered links between std/ui App pages only. A plain
server page (xw's search and sign in) was a full load both ways, and
forms on it always were. A search query cannot go in an App's model: the
model is shared by every viewer, so one person's query would show for
everyone. This decision makes layer 1 general, the way Turbo Drive does;
layer 3 is a later decision, designed against a real case.

## Decision

1. Pages. The runtime takes over every same-origin page that loads it:
   a std/ui App page, or a server page whose `<body>` carries
   `data-xo-page`. `ui.plain(title, body, css:, prefix:)` renders such a
   page from an `Html` body; `ui.serve_runtime(r, prefix:)` serves the
   runtime without an App and `ui.runtime_url(prefix)` names its stamped
   URL, for pages written by hand. A page that loads no runtime, or
   another build of it (its stamp differs), is a full load, so a new
   runtime always runs from a real load.
2. Links: as 0668, between any such pages. `data-xo-action="replace"`
   replaces the history entry instead of pushing one.
3. GET forms submit in place: the runtime builds the URL from the form
   (and its submitter's `formaction`, `formmethod`, name and value),
   fetches it, patches the page in and pushes the URL, so back, forward
   and a shared link work and the server renders the first load. A form
   with `data-xo-submit-on-input="MS"` also submits MS ms (default 250)
   after the last input, replacing the history entry (search as you
   type). A newer request aborts the one in flight.
4. POST forms follow Turbo's contract: the server answers a redirect
   (303), which the runtime follows and shows as a new page; a 422 is the
   form again with its errors, shown in place with the URL unchanged (a
   200 is shown the same way). `multipart/form-data` bodies are sent as
   such. If the fetch fails, or the answer is not a page the runtime
   takes over, the form is submitted for real, never repeated as a GET.
   An App's own forms (`data-xo-submit`) stay Msgs.
5. Frames: an element with an id and `data-xo-frame`, outside a running
   App's view, is a frame. A link or form with `data-xo-frame="ID"`
   fetches its URL and patches only that element from the answer (the
   URL still updates); without that element in the answer the whole page
   is patched. Back and forward patch the whole page.
6. The patch keeps the focused input's node and text (the keyed patch of
   5c377ace8), so a search box keeps focus and what is typed while
   results change around it. A push scrolls to the top, a replace keeps
   the scroll, back and forward restore it.
7. Opting out: `data-xo-reload` on a link, form or submitter, a
   `target` other than `_self`, `download`, or a modifier key.

Not in this decision: a snapshot cache shown at once on back (Turbo's
preview), prefetch on hover, and taking over pages that do not load the
runtime (Turbo's default).

## Consequences

- Measured in headless Chrome (2026-11-15, the multi-page test server:
  two counter Apps, the todo App with its wasm client, a search page, a
  sign in page and a framed list as `ui.plain` pages; driver
  ~/xo-lead/notes/stdui/e2e/drive-pages.mjs, 14/14):
  - App page to plain page and back: swapped in 1 to 6 ms with the
    window kept, and the App's clicks work after the swap;
  - typing three characters 40 ms apart sends one request, shows the
    results 306 ms after the last key (a 200 ms debounce), replaces the
    URL with `?q=app` without a history entry, and the box keeps its
    node, focus and text;
  - Enter pushes `?q=gr`; back returns to `?q=app`;
  - an empty sign in gets the 422 errors in place at `/login`; a name
    gets the 303 to `/a/` (102 ms);
  - a frame link changes the list and the URL (`?page=1`) but not the
    header outside the frame; back patches the whole page;
  - a shared `/search?q=lem` renders on the server;
  - a link to a page without the runtime is a full load.
  Navigation (17/17), todo (21/21), morph (3/3) and redeploy (6/6)
  checks pass on the same runtime.
- xw can make search, sign in and its other plain pages part of the
  page layer by adding `data-xo-page` and the runtime's script element
  (or rendering with `ui.plain`), with no change to their handlers.
- The runtime grows from 14,792 to 18,855 bytes (5,560 to 6,908 gzipped).
- Pages the runtime takes over must answer a navigation with a whole
  HTML page (no fragments): the patch picks out what changed.

## Alternatives

- Fragment answers (htmx's `HX-Request`): smaller replies, but each
  handler needs a second shape; the whole page patched gives the same
  result for the pages measured.
- Take over every same-origin page (Turbo's default): breaks pages that
  were never written for it (downloads answering HTML, pages with their
  own scripts); opt-in through the runtime's presence is safer.
- Put the query in an App model: shared by every viewer (see Context);
  per-viewer state is layer 3's decision.
