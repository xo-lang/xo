# A std/ui page in a background tab stops syncing, and a CDP test sees no update

Status: Observed (2026-11-15)

## Symptom

The examples/ui-todo browser check (headless Chrome over CDP, two pages)
failed "A sees B's toggle": page A never showed a change page B made,
although the server had it and a reload of A showed it.

## Cause

The std/ui runtime polls only while `document.hidden` is false, to spare
phones' batteries. Two pages in one headless Chrome are two tabs: only the
front one is visible, so A, behind B, never polled. The runtime also did
not sync when a page became visible again, so a real user who switched
back to the tab saw stale state until the next event.

## Fix

- The runtime syncs on `visibilitychange` when the page becomes visible
  (std/ui 1b), and on a live websocket notice whatever the visibility
  (1d), so a background tab catches up at once.
- A CDP driver that checks a page calls `Page.bringToFront` on it first;
  otherwise it tests a hidden tab, which is not what a user sees.
