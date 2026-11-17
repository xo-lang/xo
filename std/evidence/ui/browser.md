# std/ui in a browser: bundle size and time to resume (decision 0659 stage 3)

Measured 2026-11-17 (task #186, xo-4c) on the Mac Studio, headless
Chrome 157, a 390 by 844 mobile viewport, against examples/ui-todo
(its App's update and view built for wasip1/wasm by `xo build
--targets=wasip1/wasm ./client`, served by `./server`). Tree: std/ui at
a2e16c080 (origin/main 6ad739c72 plus the lease and commands commits;
neither changes the runtime script or the client). Medians of 7 loads.

The driver (`drive-measure.mjs`, CDP over the devtools socket, kept with
the other e2e drivers outside the repository) records, per load, the
bytes of each response (`Network.loadingFinished` encodedDataLength, and
the decoded body) and, from the page, the navigation timing, first
contentful paint, and "resume": navigation start until the client module
is compiled, the moment the runtime starts predicting with it (a wrapper
on `WebAssembly.compileStreaming` added before the page's scripts). Cold
loads clear the cache and disable it; warm loads keep it. "Slow 4G" is
Chrome DevTools' preset: 150 ms round trip, 1.6 Mbit/s down, 750 kbit/s
up.

## Bytes (cold)

| resource | on the wire | decoded |
|---|---:|---:|
| `/` (page: rendered view and embedded model) | 2,396 | 2,278 |
| `/_xo/ui.js` (the runtime, gzip) | 8,549 | 22,787 |
| `/_xo/app.wasm` (update and view, gzip) | 92,097 | 270,737 |
| `/_xo/sync` (twice, below) | 264 each | 155 each |
| total | 103,570 | |

Warm loads fetch only the page and the syncs: the runtime and module
URLs are stamped and cached for good (`std/ui_test.xo:the runtime and client
are served at stamped URLs, cached for good`).

## Time (ms)

| profile | first byte | DOMContentLoaded | first paint | resume |
|---|---:|---:|---:|---:|
| unthrottled, cold | 1.1 | 6.4 | 16 | 7.4 |
| unthrottled, warm | 1.0 | 4.9 | 12 | 6.0 |
| slow 4G, cold | 150.8 | 414.4 | 184 | 865.5 |
| slow 4G, warm | 150.9 | 171.5 | (not recorded) | 172.6 |

Resume on slow 4G cold, every run: 865, 866, 866, 859, 867, 861, 869.
Chrome recorded no first-contentful-paint entry for the warm slow 4G
reloads; DOMContentLoaded stands in for them.

After resume a Msg is predicted locally, with the network offline: an
add shows in the same task as its submit, median 0.7 ms over 7 adds
(4.3 ms for the first, which instantiates the module).

## What it shows

- Resume, not hydrate, holds: the server-rendered page paints at 184 ms on
  slow 4G cold, about 680 ms before the client can predict; until then a
  Msg is queued and synced, as on a thin page, so nothing waits on the
  module but local prediction.
- The module is the cost: 92 KB of the 104 KB on a cold load, and on slow
  4G its download is most of the 866 ms. A thin page (no client) loads
  the page and 8.5 KB of runtime only.
- Each load syncs twice: once when the live socket opens, and once when
  the module is compiled, which switches the page to syncs that carry the
  confirmed model (`model=1`) the predictions fold over. With no change
  both answer `same` (`std/ui_test.xo:an idle sync at the current version
  answers same, without the page`).

## Open

- No size guard yet: a test that fails when the ui-todo client grows past
  a budget would keep this number from drifting.
- One app (ui-todo); a larger App (examples/ui-shop) has not been
  measured.
