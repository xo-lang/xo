# A test client waited in the backlog of a stopped in memory server

Status: Observed (2026-10-12, batched full suite on EC2 r8g.12xlarge,
main 5e25745, cmd/xo TestNoGCTests/test_shutdown on llvm)

Symptom: in a native test binary (`xo test --no-gc`, arm64 or llvm), the
test "a server finishes the request in flight after the stop"
(internal/gen/testdata/golden/test_shutdown) failed now and then with
`fault: XO-F001: deadlock: every task is blocked and no timer is pending`,
while the Go backend passed it every time. It showed on a loaded machine
(XO_SELFHOST_JOBS=48) and not in a quiet run.

Cause: std/llrt/xo_testing.c's `xo_test_mem_begin` (the http client of a
test reaching an in memory listener) waited while the listener had no
router and was not closed, "as a real backlog does" before
`http.server(l, r).serve()` starts. But serve_mem also clears the router
when a stop is requested (then drains the requests in flight), so a
request sent after the stop waited in that backlog too. The wake that
ends the drain also woke it; whether it then failed or waited again was
a thread race: if the server's listener was already freed it failed (the
expected `Err`), if not it waited again, and nothing woke it later
(freeing a listener did not wake its waiters, and its waiter list was
freed under it). Every task then waited and no timer was left: XO-F001.
std/rt's memListener has no such state: its `served` event fires once,
and a request after that while nothing is served fails at once.

Fix (2026-10-12): the in memory listener records `served` (set by
`xo_test_mem_serve`, never cleared) and `xo_test_mem_begin` waits only
while the listener was never served and is open; afterwards a request
with no router fails at once, as on the Go backend. Freeing a listener
wakes the clients still waiting for a serve that never came, and the
last of them frees the record (`nbegin`, `freed`), so no waiter is left
on freed memory. Regression test: "a request after the stop fails at
once while the listener lives on" in test_shutdown, whose server keeps
its listener open for 1s after serve returns: before the fix the late
request failed only at 3500ms (when the listener closed) instead of at
1500ms, deterministically.

Counts (2026-10-12, `xo test --no-gc -v` of test_shutdown, Go driver,
XO_SELF_DRIVER=0): on EC2 Linux arm64 (r8g.4xlarge, 30 runs in a row
then 8 loops of 10 in parallel) before the fix, llvm failed 31 of 110
runs and arm64 9 of 110, every failure this XO-F001; on the Mac llvm
failed 3 of 140 (arm64 0 of 10). After it, no XO-F001 in 700 Linux
runs (220 on r8g.4xlarge, 480 in 12 parallel loops on c8g.4xlarge), but
17 of them still failed on a second race, the virtual clock moving
twice for one idle instant (virtual-clock-double-kick.md, fixed the
same day); with both fixes 0 of 240 llvm and 0 of 240 arm64 runs
failed in 12 parallel loops on c8g.4xlarge.

Lesson: a wait in the native test world needs a waker for every way the
state can stop changing. When porting a std/rt wait to xo_testing.c,
mirror the Go condition exactly (here, `served` fires once) rather than
deriving it from state that can return to its initial value.
