# 0407: S2: subscribing to signals beyond INT and TERM (Proc.notify)

Status: Accepted (lead, 2026-10-29: 0.35 std item S2; Xo Lang 2). Not yet
implemented.

## Context

A program reacts to SIGINT and SIGTERM only through core 7.5's two phase
shutdown (`task.stop_requested()`, `task.stopping()`), and to SIGQUIT with
the task dump (decision 0251). Gateways and exporters also need SIGHUP
(reload the configuration), SIGUSR1/SIGUSR2 (rotate logs, dump state) and
SIGWINCH (a terminal resized), as Go's `os/signal.Notify` gives them. The
0.35 survey (std-coverage.md) lists this as S2.

Natively the handler (llrt/poll.xo on_signal, installed by entry.xo for
SIGINT and SIGTERM) counts signals in one word and wakes the poller through
its self pipe; Windows' console handler maps Ctrl-C, Ctrl-Break and the
console's close to them. The Go backend uses `signal.Notify` (std/rt
main.go).

## Decision

```xo
enum Signal { Hangup, Interrupt, Quit, Terminate, User1, User2, WindowChange }
fn (p: Proc) notify(signals: List[Signal]) -> Chan[Signal] uses proc
fn (p: Proc) stop_notify(c: Chan[Signal]) uses proc
fn (p: Proc) request_stop() uses proc      // core 7.5's phase 1, as the first signal
```

- `notify` returns a channel that receives each listed signal as it
  arrives (Go's `signal.Notify` on a new channel). The channel holds one
  pending signal of each kind; a signal of a kind already pending is
  dropped, as Go drops sends to a full channel. Several subscriptions to
  one signal each receive it.
- Scoped per signal, as Go's default actions are: while any subscription
  lists `Interrupt` (or `Terminate`), that signal no longer starts 7.5's
  shutdown, and the program decides; other signals keep their default. A
  `Hangup` subscription leaves Ctrl-C's shutdown alone. `Quit`
  subscribed replaces the task dump. `stop_notify(c)` ends c's
  subscription and closes c; a signal with no subscription left gets its
  default back. Unlisted Hangup, User1, User2 and WindowChange keep the
  system's default (Hangup, User1 and User2 end the program) as Go's do.
- `request_stop()` starts phase 1 of 7.5 from the program (the stop state
  set, `stopping()` closed, servers draining), so a program that took
  over Interrupt still shuts down the same way:

  ```xo
  let sigs = os.proc.notify([proc.Signal.Interrupt, proc.Signal.Terminate, proc.Signal.Hangup])
  loop: while true {
    match sigs.recv() {
      Some(proc.Signal.Hangup) => reload(os)
      Some(_) => {
        log.info("shutting down")
        os.proc.request_stop()       // servers stop accepting and drain
        break loop
      }
      None => break loop
    }
  }
  ```

- Windows maps as Go does: Ctrl-C and Ctrl-Break are `Interrupt`; the
  console's close, a logoff and a system shutdown are `Terminate`. Hangup,
  User1, User2, Quit and WindowChange are accepted and never arrive.
- `Proc` carries the capability (`uses proc`), as `kill` does; an
  attenuated Proc may notify only when it may signal (decision 0190's
  lists). FakeProc (tests): `signal_after(d)` gains a `Signal` argument
  (default Terminate, today's behavior) and delivers to subscriptions.
- Natively the handler sets a bit per signal in a pending word (atomic
  or) and writes one byte to the poller's pipe; the poller delivers each
  set bit to the subscription table (a lock, filled by notify), or to
  7.5's default when no subscription takes it. Handlers are installed
  for a signal when it is first subscribed (Hangup, User1, User2,
  WindowChange), so an unsubscribed program keeps the system's defaults.
- Go backend: `signal.Notify` on a Go channel per subscription,
  forwarding into the Xo channel without blocking (drop when pending);
  the main.go INT/TERM watcher skips a signal while a subscription takes
  it.

## Consequences

- Programs reload on SIGHUP and control their own shutdown, matching Go.
- 7.5 gains a second way into phase 1 (`request_stop`), which servers and
  readers already honor.
- Tests: a program sends itself SIGHUP, SIGUSR1 and SIGUSR2 with
  `os.proc.kill(pid)` (os pid from S8, or a child `kill -HUP $PPID`), and
  Interrupt under a subscription does not stop it until request_stop; on
  arm64, LLVM and Go, Linux in Docker; Windows' console events in the
  approved CI run.

## Alternatives

- Deliver Interrupt/Terminate and still stop: simpler, but a program could
  not delay or veto its shutdown, which is why people subscribe; Go
  suppresses the default.
- A global `signal.notify` in std/task without a capability: signals are
  process wide state that Proc already gates (kill), so they stay there.
