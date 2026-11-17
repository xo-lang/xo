# std/ui's runtime calls a DOM method the node fakes lack

Status: Observed 2026-11-17

Symptom: internal/uiruntime's TestTwoTabsAreTwoClients, TestClosedTabQueueIsAdopted,
TestReplaceWaitsForPointerUp and the other tabs.mjs tests fail with `TypeError:
root.hasAttribute is not a function` (main red from f9ee7ac74 to the fix, task #280).

Cause: the page runtime in std/ui.xo runs in node against hand-written fake elements
(internal/uiruntime/testdata/tabs.mjs, morph.mjs). The query-subset change read
`root.hasAttribute("data-xo-subset")`; the tabs.mjs root had only getAttribute. std/ui's own
tests do not run the runtime in node, so its gate passed.

Fix: the fake root has `hasAttribute`. A change to the runtime's JavaScript in std/ui.xo runs
`go test ./internal/uiruntime/` in its gate (3 s), and a new DOM call gets its method in the
fakes in the same change.
