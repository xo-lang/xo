# 0668: std/ui moves between pages without a full load

Status: Proposed (owner session, 2026-11-15). Stub: the decision is being written.

## Context

Each std/ui page is a full document load: the runtime, its state and the
client module start again on every link between an app's pages (xw's
dashboard has 7 hosts, one per page).

## Decision

To be written: the runtime follows a same-origin link to another std/ui
page by fetching it, patching it in and moving its sync to that page,
with history; an App that routes by URL in its model comes later, if an
app needs state across pages.
