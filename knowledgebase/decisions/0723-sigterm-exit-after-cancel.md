# 0723: A program still running 1 s after its stop cancel exits with status 130

Status: Accepted (owner, 2026-11-19, xw decision #178: "Make it 1 second then"; proposed by xo-lang-1 with 5 s, task #416)

## Context

Core 7.5's two phase shutdown: the first SIGINT or SIGTERM only requests a
stop, the grace period's end (30 s) or a second signal cancels main's root
scope, and a third signal exits with status 130. The spec promised that "a
program that ignores phase 1 behaves as before, only 30 seconds later",
where before meant the signal ended the process.

Lang 3 found (2026-11-19, notes/xo-lang-3/orphan-spin-2026-11-19.md) that
a program in a CPU loop got one SIGTERM and never ended: cancellation
reaches only code that blocks or checks its context, and nothing sends a
second or third signal. An orphaned test child spun for hours.

## Decision

When main's root is canceled in a real process, an exit timer is due 1 s
later (stop.xo EXIT_AFTER). If it fires, main is still running, and the
process exits with status 130 (the third signal's status), standard
output and error flushed. The timer is armed together with the grace
timer (at the grace deadline plus 1 s, rearmed by set_grace) and at a
second signal; not inside cancel, because the grace timer's function
runs with the timers' lock held. The test world (testing.run_main) is
unchanged: no exit timer.

## Consequences

- One SIGTERM ends any native program within the grace period plus 1 s
  (31 s by default), as a supervisor (systemd, Kubernetes) expects.
- Cleanup after the cancel (defer blocks, a shielded flush) has 1 s; a
  longer one is cut short, a shielded `defer` too (its own limit,
  XO-F005, is 5 s). Before the two phases such code got no time.
- Test: internal/llvm TestNativeSignalBusyExits (arm64 and llvm), a CPU
  loop with set_grace(300ms) and one SIGTERM exits 130 after about 1.3 s;
  it timed out at 20 s before.
- wasm has no signals and is unchanged.

## Alternatives

- Restore SIG_DFL at phase 2, so a later signal kills: still needs a
  second signal, which a supervisor sending one SIGTERM never sends.
- Preemptive cancellation checks in loops: a code generation change with a
  cost in every loop, for a shutdown path.
- Exit status 143 (128 + SIGTERM): 130 matches the existing forced exit,
  and the runtime does not track which signal came first.
- 5 s (the first proposal): the owner chose 1 s.
- Make the 1 s settable (proc.set_exit_after): not needed yet.
