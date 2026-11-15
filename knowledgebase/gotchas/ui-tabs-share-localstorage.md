# Two tabs of one browser share localStorage, so they shared a std/ui client

Status: Observed 2026-11-15.

## Symptom

With examples/ui-todo open in two tabs, a click in one tab sometimes never
reached the server: no error, the queue drained, the server just did not
apply it. The headless Chrome checks of decision 0659 stage 3 lost tab B's
toggle and two of its reservations every run; the stage 1 checks had
passed 14/14 with the same runtime.

## Cause

The runtime kept its client id, next seq and queue in localStorage under
one key per page. localStorage is shared by every tab of an origin, so a
tab opened after another had saved read the same id and seq. The server
keeps one seq per client and skips a seq it has seen, so when both tabs
sent the same next seq, the second was dropped as a duplicate. The stage 1
checks opened both tabs before either saved, so each had its own id in
memory and the bug did not show; stage 3 saved at startup and it showed
every time.

## Fix

Each tab is its own client: its id in sessionStorage (per tab, kept
across a reload), its seq and queue in localStorage under `KEY|id`. A new
tab adopts the entry of a tab that closed (`pagehide` marks it free) or
was not seen for 10 minutes, so offline work survives closing the tab.
Test: internal/uiruntime runs RUNTIME_JS under Node as two tabs sharing a
localStorage against a fake server that drops seen seqs.

## Avoid

A browser test with two "clients" must open the second tab after the
first has saved state, as a person does, and check every Msg landed on the
server, not just that each page looks right.
